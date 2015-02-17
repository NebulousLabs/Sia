package consensus

import (
	"github.com/NebulousLabs/Sia/crypto"
)

// FindSpendableSiacoinInput returns a SiacoinInput that the Assistant is able
// to spend, as well as the value of the input. There is no guarantee on the
// value, it could be anything.
func (a *Assistant) FindSpendableSiacoinInput() (sci SiacoinInput, value Currency) {
	for id, output := range a.State.siacoinOutputs {
		if output.UnlockHash == a.UnlockHash {
			// Check that we haven't already spent this input.
			_, exists := a.usedOutputs[id]
			if exists {
				continue
			}

			sci = SiacoinInput{
				ParentID:         id,
				UnlockConditions: a.UnlockConditions,
			}
			value = output.Value

			// Mark the input as spent.
			a.usedOutputs[id] = struct{}{}

			return
		}
	}

	a.Tester.Fatal("could not find a spendable siacoin input")
	return
}

// AddInputToTransaction takes a transaction and adds an input that the
// assistant knows how to spend, returning the transaction and the value of the
// input that got added.
func (a *Assistant) AddSiacoinInputToTransaction(inputT Transaction, sci SiacoinInput) (t Transaction) {
	// Check that the function is being used correctly
	if sci.UnlockConditions.UnlockHash() != a.UnlockConditions.UnlockHash() {
		a.Tester.Fatal("misuse of AddSiacoinInputToTransaction - unlock conditions do not match")
	}

	// Add the input to the transaction.
	t = inputT
	t.SiacoinInputs = append(t.SiacoinInputs, sci)

	// Sign the input in an insecure way.
	tsig := TransactionSignature{
		InputID:        crypto.Hash(sci.ParentID),
		CoveredFields:  CoveredFields{},
		PublicKeyIndex: 0,
	}
	tsigIndex := len(t.Signatures)
	t.Signatures = append(t.Signatures, tsig)
	sigHash := t.SigHash(tsigIndex)
	encodedSig, err := crypto.SignHash(sigHash, a.SecretKey)
	if err != nil {
		a.Tester.Fatal(err)
	}
	t.Signatures[tsigIndex].Signature = Signature(encodedSig[:])

	return
}

// testApplySiacoinOuptut creates a transaction that spends a siacoin output.
func (a *Assistant) SiacoinOutputTransaction() (txn Transaction) {
	sci, value := a.FindSpendableSiacoinInput()
	txn = a.AddSiacoinInputToTransaction(Transaction{}, sci)
	txn.SiacoinOutputs = append(txn.SiacoinOutputs, SiacoinOutput{
		Value:      value,
		UnlockHash: a.UnlockHash,
	})
	return
}
