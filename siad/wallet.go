package siad

// wallet.go contains things like signatures and scans the blockchain for
// available funds that can be spent.

import (
	"errors"

	"github.com/NebulousLabs/Andromeda/siacore"
	"github.com/NebulousLabs/Andromeda/signatures"
)

// Contains a secret key, the spend conditions associated with that key, the
// address associated with those spend conditions, and a list of outputs that
// the wallet knows how to spend.
type Wallet struct {
	State *siacore.State

	SecretKey       signatures.SecretKey
	SpendConditions siacore.SpendConditions

	OwnedOutputs         map[siacore.OutputID]struct{} // A list of outputs spendable by this wallet.
	SpentOutputs         map[siacore.OutputID]struct{} // A list of outputs spent by this wallet which may not yet be in the blockchain.
	OpenFreezeConditions map[siacore.BlockHeight]int   // A list of all heights at which freeze conditions are being used.

	// Host variables.
	HostSettings HostAnnouncement
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

// Wallet.FreezeConditions
func (w *Wallet) FreezeConditions(unlockHeight siacore.BlockHeight) (fc siacore.SpendConditions) {
	fc = w.SpendConditions
	fc.TimeLock = unlockHeight
	return
}

// Creates a new wallet that can receive and spend coins.
func CreateWallet() (w *Wallet, err error) {
	w = new(Wallet)

	var pk signatures.PublicKey
	w.SecretKey, pk, err = signatures.GenerateKeyPair()
	w.SpendConditions.PublicKeys = append(w.SpendConditions.PublicKeys, pk)
	w.SpendConditions.NumSignatures = 1

	w.OwnedOutputs = make(map[siacore.OutputID]struct{})
	w.SpentOutputs = make(map[siacore.OutputID]struct{})
	w.OpenFreezeConditions = make(map[siacore.BlockHeight]int)

	return
}

// Scans all unspent transactions and adds the ones that are spendable by this
// wallet.
func (w *Wallet) Scan() {
	w.OwnedOutputs = make(map[siacore.OutputID]struct{})

	// Check for owned outputs from the standard SpendConditions.
	scanAddresses := make(map[siacore.CoinAddress]struct{})
	scanAddresses[w.SpendConditions.CoinAddress()] = struct{}{}

	// I'm not sure that it's the wallet's job to deal with freeze conditions.
	/*
		for height, _ := range w.OpenFreezeConditions {
			if height < State.Height() {
				freezeConditions := w.SpendConditions
				freezeConditions.TimeLock = height
				scanAddresses[freezeConditions.CoinAddress()] = struct{}{}
			}
		}
	*/

	// Get the matching set of outputs and add them to the OwnedOutputs map.
	outputs := w.State.ScanOutputs(scanAddresses)
	for _, output := range outputs {
		w.OwnedOutputs[output] = struct{}{}
	}
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
		// Check that the output has not already been assigned somewhere else.
		_, exists := w.SpentOutputs[id]
		if exists {
			continue
		}

		// Check that the output exists.
		var output siacore.Output
		output, err = w.State.Output(id)
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
		err = errors.New("insufficient funds")
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
func (w *Wallet) SignTransaction(t *siacore.Transaction) (err error) {
	for i, input := range t.Inputs {
		// If we recognize the input as something we are able to sign, we sign
		// the input.
		if input.SpendConditions.CoinAddress() == w.SpendConditions.CoinAddress() {
			txnSig := siacore.TransactionSignature{
				InputID: input.OutputID,
			}
			t.Signatures = append(t.Signatures, txnSig)

			sigHash := t.SigHash(i)
			t.Signatures[i].Signature, err = signatures.SignBytes(sigHash[:], w.SecretKey)
			if err != nil {
				return
			}
		}
	}

	return
}

// Wallet.SpendCoins creates a transaction sending 'amount' to 'address', and
// allocateding 'minerFee' as a miner fee. The transaction is submitted to the
// miner pool, but is also returned.
func (w *Wallet) SpendCoins(amount, minerFee siacore.Currency, address siacore.CoinAddress) (t siacore.Transaction, err error) {
	// Scan blockchain for outputs.
	w.Scan()

	// Add `amount` of free coins to the transaction.
	err = w.FundTransaction(amount+minerFee, &t)
	if err != nil {
		return
	}

	// Add the miner fee.
	t.MinerFees = append(t.MinerFees, minerFee)

	// Add the output to `address`.
	t.Outputs = append(t.Outputs, siacore.Output{Value: amount, SpendHash: address})

	// Sign each input.
	err = w.SignTransaction(&t)
	if err != nil {
		return
	}

	err = w.State.AcceptTransaction(t)
	if err != nil {
		return
	}

	return
}
