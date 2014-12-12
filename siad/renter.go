package main

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"

	"github.com/NebulousLabs/Andromeda/consensus"
	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/hash"
)

// FileEntry will eventually have all the information for tracking an encrypted
// and erasure coded file across many hosts. Right now it just points to a
// single host which has the whole file.
type FileEntry struct {
	Host     HostEntry              // Where to find the file.
	Contract consensus.FileContract // The contract being enforced.
}

type Renter struct {
	Files map[string]FileEntry

	sync.RWMutex
}

// RentedFiles returns a list of files that the renter is aware of.
func (e *Environment) RentedFiles() (files []string) {
	for key := range e.renter.Files {
		files = append(files, key)
	}
	return
}

// ClientFundFileContract takes a template FileContract and returns a
// partial transaction containing an input for the contract, but no signatures.
func (e *Environment) ClientProposeContract(filename, nickname string) (err error) {
	// Scan the blockchain for outputs.
	e.wallet.Scan()

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
	fileContract := consensus.FileContract{
		ContractFund:      (host.Price + host.Burn) * 5000 * consensus.Currency(info.Size()), // 5000 blocks.
		FileMerkleRoot:    merkle,
		FileSize:          uint64(info.Size()),
		Start:             e.Height() + 20,
		End:               e.Height() + 520,
		ChallengeWindow:   host.Window,
		Tolerance:         host.Tolerance,
		ValidProofPayout:  host.Price * consensus.Currency(info.Size()) * consensus.Currency(host.Window),
		ValidProofAddress: host.CoinAddress,
		MissedProofPayout: host.Burn * consensus.Currency(info.Size()) * consensus.Currency(host.Window),
		// MissedProofAddress is going to be 0, funds sent to the burn address.
	}

	// Fund the client portion of the transaction.
	var t consensus.Transaction
	t.MinerFees = append(t.MinerFees, 10)
	t.FileContracts = append(t.FileContracts, fileContract)
	err = e.wallet.FundTransaction(host.Price*5010*consensus.Currency(fileContract.FileSize), &t)
	if err != nil {
		return
	}

	// Sign the transacion.
	coveredFields := consensus.CoveredFields{
		MinerFees: []uint64{0},
		Contracts: []uint64{0},
	}
	for i := range t.Inputs {
		coveredFields.Inputs = append(coveredFields.Inputs, uint64(i))
	}
	for i := range t.Inputs {
		err = e.wallet.SignTransaction(&t, coveredFields, i)
		if err != nil {
			return
		}
	}

	// Negotiate the contract to the host.
	err = host.IPAddress.Call("NegotiateContract", func(conn net.Conn) error {
		// send contract
		if _, err := encoding.WriteObject(conn, t); err != nil {
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
func (e *Environment) Download(nickname, filename string) (err error) {
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

func CreateRenter() (r *Renter) {
	r = new(Renter)
	r.Files = make(map[string]FileEntry)
	return
}
