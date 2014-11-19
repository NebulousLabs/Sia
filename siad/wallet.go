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
	SecretKey       signatures.SecretKey
	SpendConditions siacore.SpendConditions

	OwnedOutputs         map[siacore.OutputID]siacore.Output // All outputs to CoinAddress
	SpentOutputs         map[siacore.OutputID]siacore.Output // A list of outputs that have been assigned to transactions, though the transactions may not be in a block yet.
	OpenFreezeConditions map[siacore.BlockHeight]int         // A list of all heights at which freeze conditions are being used.

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

	w.OwnedOutputs = make(map[siacore.OutputID]siacore.Output)
	w.SpentOutputs = make(map[siacore.OutputID]siacore.Output)
	w.OpenFreezeConditions = make(map[siacore.BlockHeight]int)

	return
}

// Scans all unspent transactions and adds the ones that are spendable by this
// wallet.
func (w *Wallet) Scan(state *siacore.State) {
	w.OwnedOutputs = make(map[siacore.OutputID]siacore.Output)

	// Check for owned outputs from the standard SpendConditions.
	for id, output := range state.UnspentOutputs {
		if output.SpendHash == w.SpendConditions.CoinAddress() {
			w.OwnedOutputs[id] = output
		}
	}

	// Check for spendable outputs from the freeze conditions.
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
	for id, output := range w.OwnedOutputs {
		if total >= amount {
			break
		}

		// Check that the output has not already been assigned somewhere else.
		_, exists := w.SpentOutputs[id]
		if exists {
			continue
		}

		// Create an input to add to the transaction.
		newInput := siacore.Input{
			OutputID:        id,
			SpendConditions: w.SpendConditions,
		}
		newInputs = append(newInputs, newInput)

		total += output.Value
	}

	// Check that the sum of the inputs is sufficient to complete the
	// transaction.
	if total < amount {
		err = errors.New("insufficient funds")
		return
	}

	// Add all of the inputs to the transaction.
	t.Inputs = append(t.Inputs, newInputs...)

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
func (w *Wallet) SpendCoins(amount, minerFee siacore.Currency, address siacore.CoinAddress, state *siacore.State) (t siacore.Transaction, err error) {
	// Scan blockchain for outputs.
	w.Scan(state)

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

	err = state.AcceptTransaction(t)
	if err != nil {
		return
	}

	return
}
