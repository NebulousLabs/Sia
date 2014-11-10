package sia

// wallet.go contains things like signatures and scans the blockchain for
// available funds that can be spent.

import (
	"crypto/ecdsa"
	"errors"
)

// Contains a secret key, the spend conditions associated with that key, the
// address associated with those spend conditions, and a list of outputs that
// the wallet knows how to spend.
type Wallet struct {
	SecretKey       *ecdsa.PrivateKey
	SpendConditions SpendConditions

	OwnedOutputs         map[OutputID]Output // All outputs to CoinAddress
	SpentOutputs         map[OutputID]Output // A list of outputs that have been assigned to transactions, though the transactions may not be in a block yet.
	OpenFreezeConditions map[BlockHeight]int // A list of all heights at which freeze conditions are being used.
}

// Most of the parameters are already in the file contract, but what's not
// specified is how much of the ContractFund comes from the client, and how
// much comes from the host. This specifies how much the client is to add to
// the contract.
type FileContractParameters struct {
	Transaction        Transaction
	FileContractIndex  int
	ClientContribution Currency
}

// Wallet.FreezeConditions
func (w *Wallet) FreezeConditions(unlockHeight BlockHeight) (fc SpendConditions) {
	fc = w.SpendConditions
	fc.TimeLock = unlockHeight
	return
}

// Creates a new wallet that can receive and spend coins.
func CreateWallet() (w *Wallet, err error) {
	w = new(Wallet)

	var pk PublicKey
	w.SecretKey, pk, err = GenerateKeyPair()
	w.SpendConditions.PublicKeys = append(w.SpendConditions.PublicKeys, pk)
	w.SpendConditions.NumSignatures = 1

	w.OwnedOutputs = make(map[OutputID]Output)
	w.SpentOutputs = make(map[OutputID]Output)
	w.OpenFreezeConditions = make(map[BlockHeight]int)

	return
}

// Scans all unspent transactions and adds the ones that are spendable by this
// wallet.
func (w *Wallet) Scan(state *State) {
	w.OwnedOutputs = make(map[OutputID]Output)

	// Check for owned outputs from the standard SpendConditions.
	for id, output := range state.ConsensusState.UnspentOutputs {
		if output.SpendHash == w.SpendConditions.CoinAddress() {
			w.OwnedOutputs[id] = output
		}
	}

	// Check for spendable outputs from the freeze conditions.
}

// fundTransaction() adds `amount` Currency to the inputs, creating a refund
// output for any excess.
func (w *Wallet) FundTransaction(amount Currency, t *Transaction) (err error) {
	// Check that a nonzero amount of coins is being sent.
	if amount == Currency(0) {
		err = errors.New("cannot send 0 coins")
		return
	}

	// Add to the list of inputs until enough funds have been allocated.
	total := Currency(0)
	var newInputs []Input
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
		newInput := Input{
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
		t.Outputs = append(t.Outputs, Output{Value: total - amount, SpendHash: w.SpendConditions.CoinAddress()})
	}

	return
}

// Wallet.signTransaction() takes a transaction and adds a signature for every input
// that the wallet understands how to spend.
func (w *Wallet) SignTransaction(t *Transaction) (err error) {
	for i, input := range t.Inputs {
		// If we recognize the input as something we are able to sign, we sign
		// the input.
		if input.SpendConditions.CoinAddress() == w.SpendConditions.CoinAddress() {
			txnSig := TransactionSignature{
				InputID: input.OutputID,
			}
			t.Signatures = append(t.Signatures, txnSig)

			sigHash := t.SigHash(i)
			t.Signatures[i].Signature, err = SignBytes(sigHash[:], w.SecretKey)
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
func (w *Wallet) SpendCoins(amount, minerFee Currency, address CoinAddress, state *State) (t Transaction, err error) {
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
	t.Outputs = append(t.Outputs, Output{Value: amount, SpendHash: address})

	// Sign each input.
	err = w.SignTransaction(&t)
	if err != nil {
		return
	}

	err = state.AcceptTransaction(t)

	return
}

// Wallet.ClientFundFileContract() takes a template FileContract and returns a
// partial transaction containing an input for the contract, but no signatures.
func (w *Wallet) ClientFundFileContract(params *FileContractParameters, state *State) (err error) {
	// Scan the blockchain for outputs.
	w.Scan(state)

	// Add money to the transaction to fund the client's portion of the contract fund.
	err = w.FundTransaction(params.ClientContribution, &params.Transaction)
	if err != nil {
		return
	}

	return
}
