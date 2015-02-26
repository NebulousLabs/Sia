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

// createContractTransaction takes contract terms and a merkle root and uses
// them to build a transaction containing a file contract that satisfies the
// terms, including providing an input balance. The transaction does not get
// signed.
func (r *Renter) createContractTransaction(terms modules.ContractTerms, merkleRoot crypto.Hash) (txn consensus.Transaction, id string, err error) {
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
	id, err = r.wallet.RegisterTransaction(txn)
	if err != nil {
		return
	}
	_, err = r.wallet.FundTransaction(id, clientCost)
	if err != nil {
		return
	}
	txn, _, err = r.wallet.AddFileContract(id, contract)
	if err != nil {
		return
	}

	return
}

// negotiateContract creates a file contract for a host according to the
// requests of the host. There is an assumption that only hosts with acceptable
// terms will be put into the hostdb.
func (r *Renter) negotiateContract(host modules.HostEntry, up modules.UploadParams) (contract consensus.FileContract, fcid consensus.FileContractID, err error) {
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

	// Create the contract terms.
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

	// Create the transaction holding the contract. This is done first so the
	// transaction is created sooner, which will impact the user's wallet
	// balance faster vs. waiting for the whole thing to upload before
	// affecting the user's balance.
	merkleRoot, err := crypto.ReaderMerkleRoot(up.Data, filesize)
	if err != nil {
		return
	}
	unsignedTxn, txnRef, err := r.createContractTransaction(terms, merkleRoot)
	if err != nil {
		return
	}
	up.Data.Seek(0, 0)

	// TODO: This is a hackish sleep, we need to be certain that all dependent
	// transactions have propgated to the host's transaction pool. Instead,
	// built into the protocol should be a step where any dependent
	// transactions are automatically provided.
	time.Sleep(time.Second * 60)

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
		if response != modules.AcceptTermsResponse {
			return errors.New(response)
		}

		// Set a timeout for the contract that assumes a minimum connection of
		// 64kbps, then send the data that the host will be storing.
		conn.SetDeadline(time.Now().Add(time.Duration(filesize) * 128 * time.Microsecond))
		_, err = io.CopyN(conn, up.Data, int64(filesize))
		if err != nil {
			return
		}

		// Send the unsigned transaction to the host.
		_, err = encoding.WriteObject(conn, unsignedTxn)
		if err != nil {
			return
		}

		// The host will respond with a transaction with the collateral added.
		// Add the collateral inputs from the host to the original wallet
		// transaction.
		var collateralTxn consensus.Transaction
		err = encoding.ReadObject(conn, &collateralTxn, 16e3)
		if err != nil {
			return
		}
		for i := len(unsignedTxn.SiacoinInputs); i < len(collateralTxn.SiacoinInputs); i++ {
			_, _, err = r.wallet.AddSiacoinInput(txnRef, collateralTxn.SiacoinInputs[i])
			if err != nil {
				return
			}
		}
		signedTxn, err := r.wallet.SignTransaction(txnRef, true)
		if err != nil {
			return
		}

		// Send the signed transaction back to the host.
		_, err = encoding.WriteObject(conn, signedTxn)
		if err != nil {
			return
		}

		fcid = signedTxn.FileContractID(0)

		// TODO: We don't actually watch the blockchain to make sure that the
		// file contract made it.

		return
	})

	return
}
