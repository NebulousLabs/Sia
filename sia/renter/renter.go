package renter

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/hash"
	"github.com/NebulousLabs/Sia/sia/components"
)

type FilePiece struct {
	Host     components.HostEntry   // Where to find the file.
	Contract consensus.FileContract // The contract being enforced.
}

type FileEntry struct {
	Pieces []FilePiece
}

type Renter struct {
	state  *consensus.State
	files  map[string]FileEntry
	hostDB components.HostDB
	wallet components.Wallet
	rwLock sync.RWMutex
}

func New(state *consensus.State, hdb components.HostDB, wallet components.Wallet) (r *Renter) {
	r = &Renter{
		state:  state,
		hostDB: hdb,
		wallet: wallet,
		files:  make(map[string]FileEntry),
	}
	return
}

// ClientFundFileContract takes a template FileContract and returns a
// partial transaction containing an input for the contract, but no signatures.
//
// TODO: We need to get the id of the contract before we can start doing
// re-uploading.
func (r *Renter) proposeContract(filename string, duration consensus.BlockHeight) (fp FilePiece, err error) {
	// Open the file, create a merkle hash.
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return
	}
	merkle, err := hash.ReaderMerkleRoot(file, hash.CalculateSegments(uint64(info.Size())))
	if err != nil {
		return
	}
	// reset read position
	if _, err = file.Seek(0, 0); err != nil {
		return
	}

	// Find a host.
	host, err := r.hostDB.RandomHost()
	if err != nil {
		return
	}

	// Fill out the contract according to the whims of the host.
	// The contract fund: (burn * duration + price * full duration) * filesize
	delay := consensus.BlockHeight(20)
	contractFund := ((host.Price * consensus.Currency(duration+delay)) + host.Burn*consensus.Currency(duration)) * consensus.Currency(info.Size())
	fileContract := consensus.FileContract{
		ContractFund:       contractFund,
		FileMerkleRoot:     merkle,
		FileSize:           uint64(info.Size()),
		Start:              r.state.Height() + delay,
		End:                r.state.Height() + duration + delay,
		ChallengeWindow:    host.Window,
		Tolerance:          host.Tolerance,
		ValidProofPayout:   host.Price * consensus.Currency(info.Size()) * consensus.Currency(host.Window),
		ValidProofAddress:  host.CoinAddress,
		MissedProofPayout:  host.Burn * consensus.Currency(info.Size()) * consensus.Currency(host.Window),
		MissedProofAddress: consensus.CoinAddress{}, // The empty address is the burn address.
	}

	// Fund the client portion of the transaction.
	minerFee := consensus.Currency(10) // TODO: ask wallet.
	renterPortion := host.Price * consensus.Currency(duration) * consensus.Currency(fileContract.FileSize)
	id, err := r.wallet.RegisterTransaction(consensus.Transaction{})
	if err != nil {
		return
	}
	err = r.wallet.FundTransaction(id, renterPortion+minerFee)
	if err != nil {
		return
	}
	err = r.wallet.AddMinerFee(id, minerFee)
	if err != nil {
		return
	}
	err = r.wallet.AddFileContract(id, fileContract)
	if err != nil {
		return
	}
	transaction, err := r.wallet.SignTransaction(id, false)
	if err != nil {
		return
	}

	// Negotiate the contract to the host.
	err = host.IPAddress.Call("NegotiateContract", func(conn net.Conn) error {
		// send contract
		if _, err := encoding.WriteObject(conn, transaction); err != nil {
			return err
		}
		// read response
		var response string
		if err := encoding.ReadObject(conn, &response, 128); err != nil {
			return err
		}
		if response != components.AcceptContractResponse {
			return errors.New(response)
		}
		// host accepted, so transmit file data
		// (no prefix needed, since FileSize is included in the metadata)
		_, err = io.CopyN(conn, file, info.Size())
		return err
	})
	if err != nil {
		return
	}

	// Record the file in to the renter database.
	fp = FilePiece{
		Host:     host,
		Contract: fileContract,
	}

	return
}

func (r *Renter) RenameFile(currentName, newName string) error {
	// Check that the currentName exists and the newName doesn't.
	entry, exists := r.files[currentName]
	if !exists {
		return errors.New("no file found by that name")
	}
	_, exists = r.files[newName]
	if exists {
		return errors.New("file of new name already exists")
	}

	// Do the renaming.
	delete(r.files, currentName)
	r.files[newName] = entry
	return nil
}

// TODO: Do the uploading in parallel.
func (r *Renter) RentFile(filename, nickname string, totalPieces, requiredPieces, optimalRepairPieces int) (err error) {
	r.lock()
	defer r.unlock()

	_, exists := r.files[nickname]
	if exists {
		return errors.New("file of that nickname already exists")
	}

	// Make an entry for this file.
	var pieces []FilePiece
	for i := 0; i < totalPieces; i++ {
		var piece FilePiece
		piece, err = r.proposeContract(filename, consensus.BlockHeight(2000+1000*i))
		if err != nil {
			i--
			fmt.Println(err)
			// TODO: Something more in the error case. Maybe let the hostdb know that this host is having problems?
		}
		pieces = append(pieces, piece)
	}

	r.files[nickname] = FileEntry{Pieces: pieces}
	return
}

func (r *Renter) downloadPiece(piece FilePiece, destination string) (err error) {
	return piece.Host.IPAddress.Call("RetrieveFile", func(conn net.Conn) error {
		// send filehash
		if _, err := encoding.WriteObject(conn, piece.Contract.FileMerkleRoot); err != nil {
			return err
		}
		// TODO: read error
		// copy response into file
		file, err := os.Create(destination)
		if err != nil {
			return err
		}
		_, err = io.CopyN(file, conn, int64(piece.Contract.FileSize))
		file.Close()
		if err != nil {
			os.Remove(destination)
		}
		return err
	})
}

// Download requests a file from the host it was stored with, and downloads it
// into the specified filename.
func (r *Renter) Download(nickname, filename string) (err error) {
	entry, exists := r.files[nickname]
	if !exists {
		return errors.New("no file entry for file: " + nickname)
	}

	// We just need to get one piece, we'll keep contacting hosts until one
	// doesn't return an error.
	for _, piece := range entry.Pieces {
		err = r.downloadPiece(piece, filename)
		if err != nil {
			return
		} else {
			fmt.Println(err)
			r.hostDB.FlagHost(piece.Host.ID)
		}
	}

	return
}
