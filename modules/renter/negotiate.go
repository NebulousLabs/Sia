package renter

import (
	"errors"
	"io"
	"net"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	defaultWindowSize = 288 // 48 Hours
)

// createContractTransaction takes contract terms and a merkle root and uses
// them to build a transaction containing a file contract that satisfies the
// terms, including providing an input balance. The transaction does not get
// signed.
func (r *Renter) createContractTransaction(terms modules.ContractTerms, merkleRoot crypto.Hash) (txn types.Transaction, id string, err error) {
	// Get the payout as set by the missed proofs, and the client fund as determined by the terms.
	var payout types.Currency
	for _, output := range terms.MissedProofOutputs {
		payout = payout.Add(output.Value)
	}

	// Get the cost to the client as per the terms in the contract.
	sizeCurrency := types.NewCurrency64(terms.FileSize)
	durationCurrency := types.NewCurrency64(uint64(terms.Duration))
	clientCost := terms.Price.Mul(sizeCurrency).Mul(durationCurrency)

	// Fill out the contract.
	contract := types.FileContract{
		FileMerkleRoot:     merkleRoot,
		FileSize:           terms.FileSize,
		WindowStart:        terms.DurationStart + terms.Duration,
		WindowEnd:          terms.DurationStart + terms.Duration + terms.WindowSize,
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
func (r *Renter) negotiateContract(host modules.HostSettings, up modules.FileUploadParams) (contract types.FileContract, fcid types.FileContractID, key crypto.TwofishKey, err error) {
	height := r.blockHeight

	key, err = crypto.GenerateTwofishKey()
	if err != nil {
		return
	}

	file, err := os.Open(up.Filename)
	if err != nil {
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return
	}

	filesize := uint64(info.Size())

	// Get the price and payout.
	sizeCurrency := types.NewCurrency64(filesize)
	durationCurrency := types.NewCurrency64(uint64(up.Duration))
	clientCost := host.Price.Mul(sizeCurrency).Mul(durationCurrency)
	hostCollateral := host.Collateral.Mul(sizeCurrency).Mul(durationCurrency)
	payout := clientCost.Add(hostCollateral)
	validOutputValue := payout.Sub(types.FileContract{Payout: payout}.Tax())

	// Create the contract terms.
	terms := modules.ContractTerms{
		FileSize:      filesize,
		Duration:      up.Duration,
		DurationStart: height - 1,
		WindowSize:    defaultWindowSize,
		Price:         host.Price,
		Collateral:    host.Collateral,

		ValidProofOutputs: []types.SiacoinOutput{
			{Value: validOutputValue, UnlockHash: host.UnlockHash},
		},

		MissedProofOutputs: []types.SiacoinOutput{
			{Value: payout, UnlockHash: types.ZeroUnlockHash},
		},
	}

	// TODO: This is a hackish sleep, we need to be certain that all dependent
	// transactions have propgated to the host's transaction pool. Instead,
	// built into the protocol should be a step where any dependent
	// transactions are automatically provided.
	time.Sleep(types.RenterZeroConfDelay)

	// Perform the negotiations with the host through a network call.
	conn, err := net.DialTimeout("tcp", string(host.IPAddress), 10e9)
	if err != nil {
		return
	}
	defer conn.Close()
	err = encoding.WriteObject(conn, [8]byte{'C', 'o', 'n', 't', 'r', 'a', 'c', 't'})
	if err != nil {
		return
	}

	// Send the contract terms and read the response.
	if err = encoding.WriteObject(conn, terms); err != nil {
		return
	}
	var response string
	if err = encoding.ReadObject(conn, &response, 128); err != nil {
		return
	}
	if response != modules.AcceptTermsResponse {
		err = errors.New(response)
		return
	}

	// Encrypt and transmit the file data while calculating its Merkle root.
	tee := io.TeeReader(
		// wrap file reader in encryption layer
		key.NewReader(file),
		// each byte we read from tee will also be written to conn
		conn,
	)
	merkleRoot, err := crypto.ReaderMerkleRoot(tee)
	if err != nil {
		return
	}

	// Create the transaction holding the contract. This is done first so the
	// transaction is created sooner, which will impact the user's wallet
	// balance faster vs. waiting for the whole thing to upload before
	// affecting the user's balance.
	unsignedTxn, txnRef, err := r.createContractTransaction(terms, merkleRoot)
	if err != nil {
		return
	}

	// Send the unsigned transaction to the host.
	err = encoding.WriteObject(conn, unsignedTxn)
	if err != nil {
		return
	}

	// The host will respond with a transaction with the collateral added.
	// Add the collateral inputs from the host to the original wallet
	// transaction.
	var collateralTxn types.Transaction
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
	err = encoding.WriteObject(conn, signedTxn)
	if err != nil {
		return
	}

	fcid = signedTxn.FileContractID(0)
	contract = signedTxn.FileContracts[0]

	// Read an ack from the host that all is well.
	var ack bool
	err = encoding.ReadObject(conn, &ack, 1)
	if err != nil {
		return
	}
	if !ack {
		err = errors.New("host negotiation failed")
		return
	}

	// TODO: We don't actually watch the blockchain to make sure that the
	// file contract made it.

	return
}
