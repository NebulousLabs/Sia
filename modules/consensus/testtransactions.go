package consensus

import (
	"bytes"
	"crypto/rand"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// FindSpendableSiacoinInput returns a SiacoinInput that the ConsensusTester is able
// to spend, as well as the value of the input. There is no guarantee on the
// value, it could be anything.
func (ct *ConsensusTester) FindSpendableSiacoinInput() (sci types.SiacoinInput, value types.Currency) {
	for id, output := range ct.siacoinOutputs {
		if output.UnlockHash == ct.UnlockHash {
			// Check that we haven't already spent this input.
			_, exists := ct.usedOutputs[id]
			if exists {
				continue
			}

			sci = types.SiacoinInput{
				ParentID:         id,
				UnlockConditions: ct.UnlockConditions,
			}
			value = output.Value

			// Mark the input as spent.
			ct.usedOutputs[id] = struct{}{}

			return
		}
	}

	ct.Fatal("could not find a spendable siacoin input")
	return
}

// AddSiacoinInputToTransaction takes a transaction and adds an input that the
// assistant knows how to spend, returning the transaction and the value of the
// input that got added.
func (ct *ConsensusTester) AddSiacoinInputToTransaction(inputT types.Transaction, sci types.SiacoinInput) (t types.Transaction) {
	// Check that the function is being used correctly
	if sci.UnlockConditions.UnlockHash() != ct.UnlockConditions.UnlockHash() {
		ct.Fatal("misuse of AddSiacoinInputToTransaction - unlock conditions do not match")
	}

	// Add the input to the transaction.
	t = inputT
	t.SiacoinInputs = append(t.SiacoinInputs, sci)

	// Sign the input in an insecure way.
	tsig := types.TransactionSignature{
		ParentID:       crypto.Hash(sci.ParentID),
		CoveredFields:  types.CoveredFields{},
		PublicKeyIndex: 0,
	}
	tsigIndex := len(t.TransactionSignatures)
	t.TransactionSignatures = append(t.TransactionSignatures, tsig)
	sigHash := t.SigHash(tsigIndex)
	encodedSig, err := crypto.SignHash(sigHash, ct.SecretKey)
	if err != nil {
		ct.Fatal(err)
	}
	t.TransactionSignatures[tsigIndex].Signature = types.Signature(encodedSig[:])

	return
}

// SiacoinOutputTransaction creates and funds a transaction that has a siacoin
// output, and returns that transaction.
func (ct *ConsensusTester) SiacoinOutputTransaction() (txn types.Transaction) {
	sci, value := ct.FindSpendableSiacoinInput()
	txn = ct.AddSiacoinInputToTransaction(types.Transaction{}, sci)
	txn.SiacoinOutputs = append(txn.SiacoinOutputs, types.SiacoinOutput{
		Value:      value,
		UnlockHash: ct.UnlockHash,
	})
	return
}

// FileContractTransaction creates and funds a transaction that has a file
// contract, and returns that transaction.
func (ct *ConsensusTester) FileContractTransaction(start types.BlockHeight, expiration types.BlockHeight) (txn types.Transaction, file []byte) {
	sci, value := ct.FindSpendableSiacoinInput()
	txn = ct.AddSiacoinInputToTransaction(types.Transaction{}, sci)

	// Create the file to make the contract from, and get the Merkle root.
	file = make([]byte, 4e3)
	_, err := rand.Read(file)
	if err != nil {
		ct.Fatal(err)
	}
	mRoot, err := crypto.ReaderMerkleRoot(bytes.NewReader(file))
	if err != nil {
		ct.Fatal(err)
	}

	// Add a full file contract to the transaction.
	txn.FileContracts = append(txn.FileContracts, types.FileContract{
		FileSize:       4e3,
		FileMerkleRoot: mRoot,
		Start:          start,
		Payout:         value,
		Expiration:     expiration,
		MissedProofOutputs: []types.SiacoinOutput{
			types.SiacoinOutput{
				Value: value,
			},
		},
		UnlockHash: ct.UnlockHash,
	})
	txn.FileContracts[0].ValidProofOutputs = []types.SiacoinOutput{types.SiacoinOutput{Value: value.Sub(txn.FileContracts[0].Tax())}}

	return
}
