package renter

import (
	"errors"
	"io"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/hash"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	// TODO: ask wallet
	minerFee = 10

	defaultWindowSize = 100
)

func (r *Renter) createContractTransaction(host modules.HostEntry, terms modules.ContractTerms, merkleRoot hash.Hash) (txn consensus.Transaction, err error) {
	// Fill out the contract according to the whims of the host.
	duration := terms.WindowSize * consensus.BlockHeight(terms.NumWindows)
	contract := consensus.FileContract{
		FileMerkleRoot:     merkleRoot,
		FileSize:           terms.FileSize,
		Start:              terms.StartHeight,
		End:                terms.StartHeight + duration,
		ValidProofAddress:  host.CoinAddress,
		MissedProofAddress: consensus.ZeroAddress, // The empty address is the burn address.
	}

	fund := host.Price*consensus.Currency(duration)*consensus.Currency(terms.FileSize) + minerFee

	// Create the transaction.
	id, err := r.wallet.RegisterTransaction(txn)
	if err != nil {
		return
	}
	err = r.wallet.FundTransaction(id, fund)
	if err != nil {
		return
	}
	err = r.wallet.AddMinerFee(id, minerFee)
	if err != nil {
		return
	}
	err = r.wallet.AddFileContract(id, contract)
	if err != nil {
		return
	}
	txn, err = r.wallet.SignTransaction(id, false)
	if err != nil {
		return
	}

	return
}

func (r *Renter) negotiateContract(host modules.HostEntry, up modules.UploadParams) (contract consensus.FileContract, err error) {
	r.state.RLock()
	height := r.state.Height()
	r.state.RUnlock()

	// get filesize via Seek
	// (these Seeks are guaranteed not to return errors)
	n, _ := up.Data.Seek(0, 2)
	filesize := uint64(n)
	up.Data.Seek(0, 0) // seek back to beginning

	// create ContractTerms
	terms := modules.ContractTerms{
		FileSize:           filesize,
		StartHeight:        height + up.Delay,
		WindowSize:         defaultWindowSize,
		NumWindows:         (uint64(up.Duration) / defaultWindowSize) + 1,
		Price:              host.Price,      // ??
		Collateral:         host.Collateral, // ??
		ValidProofAddress:  host.CoinAddress,
		MissedProofAddress: consensus.ZeroAddress,
	}

	// TODO: call r.hostDB.FlagHost(host.IPAddress) if negotiation is unsuccessful
	// (and it isn't our fault)
	err = host.IPAddress.Call("NegotiateContract", func(conn net.Conn) (err error) {
		// send ContractTerms
		if _, err = encoding.WriteObject(conn, terms); err != nil {
			return
		}
		// read response
		var response string
		if err = encoding.ReadObject(conn, &response, 128); err != nil {
			return
		}
		if response != modules.AcceptContractResponse {
			return errors.New(response)
		}

		// file transfer is going to take a while, so extend the timeout.
		// This assumes a minimum transfer rate of ~1 Mbps
		conn.SetDeadline(time.Now().Add(time.Duration(filesize) * 8 * time.Microsecond))

		// simultaneously transmit file data and calculate Merkle root
		tee := io.TeeReader(up.Data, conn)
		merkleRoot, err := hash.ReaderMerkleRoot(tee, filesize)
		if err != nil {
			return
		}
		// create and transmit transaction containing file contract
		txn, err := r.createContractTransaction(host, terms, merkleRoot)
		if err != nil {
			return
		}
		contract = txn.FileContracts[0]
		_, err = encoding.WriteObject(conn, txn)
		return
	})

	return
}
