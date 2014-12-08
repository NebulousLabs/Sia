package main

import (
	"errors"
	"io"
	"net"
	"os"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/hash"
	"github.com/NebulousLabs/Andromeda/siacore"
)

// FileEntry will eventually have all the information for tracking an encrypted
// and erasure coded file across many hosts. Right now it just points to a
// single host which has the whole file.
type FileEntry struct {
	Host     HostEntry            // Where to find the file.
	Contract siacore.FileContract // The contract being enforced.
}

type Renter struct {
	Files map[string]FileEntry
}

// ClientFundFileContract takes a template FileContract and returns a
// partial transaction containing an input for the contract, but no signatures.
func (e *Environment) ClientProposeContract(filename string) (err error) {
	// Scan the blockchain for outputs.
	e.wallet.Scan()

	// Open the file, create a merkle hash.
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	info, err := file.Stat()
	if err != nil {
		return
	}
	merkle, err := hash.ReaderMerkleRoot(file, hash.CalculateSegments(uint64(info.Size())))
	if err != nil {
		return
	}

	// Find a host.
	host, err := e.hostDatabase.ChooseHost()
	if err != nil {
		return
	}

	// Fill out the contract according to the whims of the host.
	fileContract := siacore.FileContract{
		ContractFund:      (host.Price + host.Burn) * 5000 * siacore.Currency(info.Size()), // 5000 blocks.
		FileMerkleRoot:    merkle,
		FileSize:          uint64(info.Size()),
		Start:             e.Height() + 100,
		End:               e.Height() + 5100,
		ChallengeWindow:   host.Window,
		Tolerance:         host.Tolerance,
		ValidProofPayout:  host.Price,
		ValidProofAddress: host.CoinAddress,
		MissedProofPayout: host.Burn,
		// MissedProofAddress is going to be 0, funds sent to the burn address.
	}

	// Fund the client portion of the transaction.
	var t siacore.Transaction
	t.MinerFees = append(t.MinerFees, 10)
	t.FileContracts = append(t.FileContracts, fileContract)
	err = e.wallet.FundTransaction(host.Price*5010*siacore.Currency(fileContract.FileSize), &t)
	if err != nil {
		return
	}

	// Sign the transacion.
	coveredFields := siacore.CoveredFields{
		MinerFees: []uint64{0},
		Contracts: []uint64{0},
	}
	for i := range t.Inputs {
		coveredFields.Inputs = append(coveredFields.Inputs, uint64(i))
	}
	e.wallet.SignTransaction(&t, coveredFields)

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
		_, err := io.Copy(conn, file)
		return err
	})
	if err != nil {
		return
	}

	// Record the file in to the renter database.
	e.renter.Files[filename] = FileEntry{
		Host:     host,
		Contract: fileContract,
	}

	return
}

// Download requests a file from the host it was stored with, and downloads it
// into the specified filename.
func (e *Environment) Download(filename string) (err error) {
	fe, ok := e.renter.Files[filename]
	if !ok {
		return errors.New("no file entry for file: " + filename)
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
		_, err = io.Copy(file, conn)
		return err
	})
}

func CreateRenter() (r *Renter) {
	r = new(Renter)
	r.Files = make(map[string]FileEntry)
	return
}
