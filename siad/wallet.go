package main

// wallet.go contains things like signatures and scans the blockchain for
// available funds that can be spent.

import (
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/siacore"
	"github.com/NebulousLabs/Andromeda/signatures"
)

// Contains a secret key, the spend conditions associated with that key, the
// address associated with those spend conditions, and a list of outputs that
// the wallet knows how to spend.
type Wallet struct {
	state *siacore.State

	SecretKey       signatures.SecretKey
	SpendConditions siacore.SpendConditions

	OwnedOutputs map[siacore.OutputID]struct{} // A list of outputs spendable by this wallet.
	SpentOutputs map[siacore.OutputID]struct{} // A list of outputs spent by this wallet which may not yet be in the blockchain.
}

// Most of the parameters are already in the file contract, but what's not
// specified is how much of the ContractFund comes from the client, and how
// much comes from the host. This specifies how much the client is to add to
// the contract.
type FileContractParameters struct {
	Transaction        siacore.Transaction
	FileContractIndex  int
	ClientContribution siacore.Currency
}

// Creates a new wallet that can receive and spend coins.
func CreateWallet(s *siacore.State) *Wallet {
	w := &Wallet{
		state:        s,
		OwnedOutputs: make(map[siacore.OutputID]struct{}),
		SpentOutputs: make(map[siacore.OutputID]struct{}),
	}

	sk, pk, err := signatures.GenerateKeyPair()
	if err != nil {
		panic(err)
	}
	w.SecretKey = sk
	w.SpendConditions.PublicKeys = append(w.SpendConditions.PublicKeys, pk)
	w.SpendConditions.NumSignatures = 1

	return w
}

// Scans all unspent transactions and adds the ones that are spendable by this
// wallet.
func (w *Wallet) Scan() {
	w.OwnedOutputs = make(map[siacore.OutputID]struct{})

	// Check for owned outputs from the standard SpendConditions.
	scanAddresses := make(map[siacore.CoinAddress]struct{})
	scanAddresses[w.SpendConditions.CoinAddress()] = struct{}{}

	// Get the matching set of outputs and add them to the OwnedOutputs map.
	w.state.RLock()
	for _, output := range w.state.ScanOutputs(scanAddresses) {
		w.OwnedOutputs[output] = struct{}{}
	}
	w.state.RUnlock()
}

// fundTransaction() adds `amount` Currency to the inputs, creating a refund
// output for any excess.
func (w *Wallet) FundTransaction(amount siacore.Currency, t *siacore.Transaction) (err error) {
	// Check that a nonzero amount of coins is being sent.
	if amount == siacore.Currency(0) {
		err = errors.New("cannot send 0 coins")
		return
	}

	// Add to the list of inputs until enough funds have been allocated.
	total := siacore.Currency(0)
	var newInputs []siacore.Input
	for id, _ := range w.OwnedOutputs {
		_, exists := w.SpentOutputs[id]
		if exists {
			continue
		}

		// Check that the output exists.
		var output siacore.Output
		output, err = w.state.Output(id)
		if err != nil {
			continue
		}

		// Create an input to add to the transaction.
		newInput := siacore.Input{
			OutputID:        id,
			SpendConditions: w.SpendConditions,
		}
		newInputs = append(newInputs, newInput)

		// Add the value of the output to the total and see if we've hit a
		// sufficient amount.
		total += output.Value
		if total >= amount {
			break
		}
	}

	// Check that the sum of the inputs is sufficient to complete the
	// transaction.
	if total < amount {
		err = fmt.Errorf("insufficient funds: %v, requested %v", total, amount)
		return
	}

	// Add all of the inputs to the transaction.
	t.Inputs = append(t.Inputs, newInputs...)

	// Add all of the inputs to the spent outputs map.
	for _, input := range newInputs {
		w.SpentOutputs[input.OutputID] = struct{}{}
	}

	// Add a refund output to the transaction if needed.
	if total-amount > 0 {
		t.Outputs = append(t.Outputs, siacore.Output{Value: total - amount, SpendHash: w.SpendConditions.CoinAddress()})
	}

	return
}

// Wallet.signTransaction() takes a transaction and adds a signature for every input
// that the wallet understands how to spend.
func (w *Wallet) SignTransaction(t *siacore.Transaction, cf siacore.CoveredFields) (err error) {
	for _, input := range t.Inputs {
		// If we recognize the input as something we are able to sign, we sign
		// the input.
		if input.SpendConditions.CoinAddress() == w.SpendConditions.CoinAddress() {
			txnSig := siacore.TransactionSignature{
				InputID:       input.OutputID,
				CoveredFields: cf,
			}
			t.Signatures = append(t.Signatures, txnSig)

			sigHash := t.SigHash(len(t.Signatures) - 1)
			t.Signatures[len(t.Signatures)-1].Signature, err = signatures.SignBytes(sigHash[:], w.SecretKey)
			if err != nil {
				return
			}
		}
	}

	return
}

// Wallet.SpendCoins creates a transaction sending 'amount' to 'dest', and
// allocateding 'minerFee' as a miner fee. The transaction is submitted to the
// miner pool, but is also returned.
func (e *Environment) SpendCoins(amount, minerFee siacore.Currency, dest siacore.CoinAddress) (t siacore.Transaction, err error) {
	// Scan blockchain for outputs.
	e.wallet.Scan()

	// Add `amount` + `minerFee` coins to the transaction.
	err = e.wallet.FundTransaction(amount+minerFee, &t)
	if err != nil {
		return
	}

	// Add the miner fee.
	t.MinerFees = append(t.MinerFees, minerFee)

	// Add the output to `dest`.
	t.Outputs = append(t.Outputs, siacore.Output{Value: amount, SpendHash: dest})

	// Sign each input.
	err = e.wallet.SignTransaction(&t, siacore.CoveredFields{WholeTransaction: true})
	if err != nil {
		return
	}

	// Send the transaction to the environment.
	e.AcceptTransaction(t)

	return
}

// WalletBalance counts up the total number of coins that the wallet knows how
// to spend, according to the State. WalletBalance will ignore all unconfirmed
// transactions that have been created.
func (e *Environment) WalletBalance() siacore.Currency {
	e.wallet.Scan()

	total := siacore.Currency(0)
	for id, _ := range e.wallet.OwnedOutputs {
		// Check that the output exists.
		var output siacore.Output
		output, err := e.state.Output(id)
		if err != nil {
			continue
		}

		total += output.Value
	}

	return total
}

// Environment.CoinAddress returns the CoinAddress which foreign coins should
// be sent to.
func (e *Environment) CoinAddress() siacore.CoinAddress {
	return e.wallet.SpendConditions.CoinAddress()
}

// SaveCoinAddress saves the address of the wallet used within the environment.
func (e *Environment) SaveCoinAddress(filename string) (err error) {
	pubKeyBytes := encoding.Marshal(e.wallet.SpendConditions.CoinAddress())

	// Open the file and write the key to the filename.
	err = ioutil.WriteFile(filename, pubKeyBytes, 0666)
	if err != nil {
		return
	}

	return
}

func (e *Environment) SaveSecretKey(filename string) (err error) {
	return
}

func (e *Environment) LoadSecretKey(filename string) (err error) {
	return
}
