package renter

import (
	// "errors"
	// "io"
	// "net"
	// "os"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	// "github.com/NebulousLabs/Sia/encoding"
	// "github.com/NebulousLabs/Sia/hash"
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
	Files  map[string]FileEntry
	hostDB components.HostDB
	rwLock sync.RWMutex
}

func New() (r *Renter) {
	r = new(Renter)
	r.Files = make(map[string]FileEntry)
	return
}

func (r *Renter) UpdateRenter(update components.RenterUpdate) error {
	r.lock()
	defer r.unlock()
	r.hostDB = update.HostDB
	return nil
}

/*
// ClientFundFileContract takes a template FileContract and returns a
// partial transaction containing an input for the contract, but no signatures.
func (e *Core) ClientProposeContract(filename, nickname string) (err error) {
	// Find a host.
	host, err := e.hostDatabase.ChooseHost()
	if err != nil {
		return
	}

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

	// Fill out the contract according to the whims of the host.
	duration := consensus.BlockHeight(500)
	delay := consensus.BlockHeight(20)
	fileContract := consensus.FileContract{
		ContractFund:      (host.Price + host.Burn) * consensus.Currency(duration) * consensus.Currency(info.Size()),
		FileMerkleRoot:    merkle,
		FileSize:          uint64(info.Size()),
		Start:             e.Height() + delay,
		End:               e.Height() + duration + delay,
		ChallengeWindow:   host.Window,
		Tolerance:         host.Tolerance,
		ValidProofPayout:  host.Price * consensus.Currency(info.Size()) * consensus.Currency(host.Window),
		ValidProofAddress: host.CoinAddress,
		MissedProofPayout: host.Burn * consensus.Currency(info.Size()) * consensus.Currency(host.Window),
		// MissedProofAddress is going to be 0, funds sent to the burn address.
	}

	// Fund the client portion of the transaction.
	minerFee := consensus.Currency(10) // TODO: ask wallet.
	renterPortion := host.Price * consensus.Currency(duration) * consensus.Currency(fileContract.FileSize)
	id, err := e.wallet.RegisterTransaction(consensus.Transaction{})
	if err != nil {
		return
	}
	err = e.wallet.FundTransaction(id, renterPortion+minerFee)
	if err != nil {
		return
	}
	err = e.wallet.AddMinerFee(id, minerFee)
	if err != nil {
		return
	}
	err = e.wallet.AddFileContract(id, fileContract)
	if err != nil {
		return
	}
	transaction, err := e.wallet.SignTransaction(id, false)
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
		if response != AcceptContractResponse {
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
	e.renter.Files[nickname] = FileEntry{
		Host:     host,
		Contract: fileContract,
	}

	return
}

// Download requests a file from the host it was stored with, and downloads it
// into the specified filename.
func (e *Core) Download(nickname, filename string) (err error) {
	fe, ok := e.renter.Files[nickname]
	if !ok {
		return errors.New("no file entry for file: " + nickname)
	}
	return fe.Host.IPAddress.Call("RetrieveFile", func(conn net.Conn) error {
		// send filehash
		if _, err := encoding.WriteObject(conn, fe.Contract.FileMerkleRoot); err != nil {
			return err
		}
		// TODO: read error
		// copy response into file
		file, err := os.Create(filename)
		if err != nil {
			return err
		}
		_, err = io.CopyN(file, conn, int64(fe.Contract.FileSize))
		file.Close()
		if err != nil {
			os.Remove(filename)
		}
		return err
	})
}
*/
