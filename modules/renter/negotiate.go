package renter

import (
	"errors"
	"io"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	defaultWindowSize = 288 // 48 Hours
)

// createFileContractTransaction creates a transaction containing a file
// contract that is aimed at negotiating with hosts.
func (r *Renter) createContractTransaction(terms modules.ContractTerms, merkleRoot crypto.Hash) (txn consensus.Transaction, err error) {
	// Get the payout as set by the missed proofs, and the client fund as determined by the terms.
	var payout consensus.Currency
	for _, output := range terms.MissedProofOutputs {
		payout = payout.Add(output.Value)
	}

	// Get the cost to the client as per the terms in the contract.
	sizeCurrency := consensus.NewCurrency64(terms.FileSize)
	durationCurrency := consensus.NewCurrency64(uint64(terms.Duration))
	clientCost := terms.Price.Mul(sizeCurrency).Mul(durationCurrency)

	// Fill out the contract.
	contract := consensus.FileContract{
		FileMerkleRoot:     merkleRoot,
		FileSize:           terms.FileSize,
		Start:              terms.DurationStart + terms.Duration,
		Expiration:         terms.DurationStart + terms.Duration + terms.WindowSize,
		Payout:             payout,
		ValidProofOutputs:  terms.ValidProofOutputs,
		MissedProofOutputs: terms.MissedProofOutputs,
	}

	// Create the transaction.
	id, err := r.wallet.RegisterTransaction(txn)
	if err != nil {
		return
	}
	err = r.wallet.FundTransaction(id, clientCost)
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

// negotiateContract creates a file contract for a host according to the
// requests of the host. There is an assumption that only hosts with acceptable
// terms will be put into the hostdb.
func (r *Renter) negotiateContract(host modules.HostEntry, up modules.UploadParams) (contract consensus.FileContract, err error) {
	height := r.state.Height()

	// Get the filesize by seeking to the end, grabbing the index, then seeking
	// back to the beginning. These calls are guaranteed not to return errors.
	n, _ := up.Data.Seek(0, 2)
	filesize := uint64(n)
	up.Data.Seek(0, 0)

	// Get the price and payout.
	sizeCurrency := consensus.NewCurrency64(filesize)
	durationCurrency := consensus.NewCurrency64(uint64(up.Duration))
	clientCost := host.Price.Mul(sizeCurrency).Mul(durationCurrency)
	hostCollateral := host.Collateral.Mul(sizeCurrency).Mul(durationCurrency)
	payout := clientCost.Add(hostCollateral)
	validOutputValue := payout.Sub(consensus.FileContract{Payout: payout}.Tax())

	// create ContractTerms
	terms := modules.ContractTerms{
		FileSize:      filesize,
		Duration:      up.Duration,
		DurationStart: height - 1,
		WindowSize:    defaultWindowSize,
		Price:         host.Price,
		Collateral:    host.Collateral,
	}
	terms.ValidProofOutputs = []consensus.SiacoinOutput{
		consensus.SiacoinOutput{
			Value:      validOutputValue,
			UnlockHash: host.UnlockHash,
		},
	}
	terms.MissedProofOutputs = []consensus.SiacoinOutput{
		consensus.SiacoinOutput{
			Value:      payout,
			UnlockHash: consensus.ZeroUnlockHash,
		},
	}

	// Perform the negotiations with the host through a network call.
	err = host.IPAddress.Call("NegotiateContract", func(conn net.Conn) (err error) {
		// Send the contract terms and read the response.
		if _, err = encoding.WriteObject(conn, terms); err != nil {
			return
		}
		var response string
		if err = encoding.ReadObject(conn, &response, 128); err != nil {
			return
		}
		if response != modules.AcceptContractResponse {
			return errors.New(response)
		}

		// Set a timeout for the contract that assumes a minimum connection of
		// 64kbps.
		conn.SetDeadline(time.Now().Add(time.Duration(filesize) * 128 * time.Microsecond))

		// Simultaneously transmit file data and calculate Merkle root.
		tee := io.TeeReader(up.Data, conn)
		merkleRoot, err := crypto.ReaderMerkleRoot(tee, filesize)
		if err != nil {
			return
		}

		// Create and transmit transaction containing the file contract.
		txn, err := r.createContractTransaction(terms, merkleRoot)
		if err != nil {
			return
		}
		contract = txn.FileContracts[0]
		_, err = encoding.WriteObject(conn, txn)
		return

		// TODO: Need some way to determine if the contract has succeeded or
		// failed. This is tricky because the host can do a few things here.
		// The safe way to handle this is to assume that the contract has
		// succeeded and then check back in a few blocks. If the contract isn't
		// in the blockchain at that point, we'll spend the output that we used
		// to fund the file contract, which will prevent the host from
		// submitting the file contract. We'll then need to upload this piece
		// somewhere else.
	})

	return
}
