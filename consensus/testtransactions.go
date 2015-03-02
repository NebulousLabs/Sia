package consensus

import (
	"bytes"
	"crypto/rand"

	"github.com/NebulousLabs/Sia/crypto"
)

// FindSpendableSiacoinInput returns a SiacoinInput that the ConsensusTester is able
// to spend, as well as the value of the input. There is no guarantee on the
// value, it could be anything.
func (ct *ConsensusTester) FindSpendableSiacoinInput() (sci SiacoinInput, value Currency) {
	for id, output := range ct.siacoinOutputs {
		if output.UnlockHash == ct.UnlockHash {
			// Check that we haven't already spent this input.
			_, exists := ct.usedOutputs[id]
			if exists {
				continue
			}

			sci = SiacoinInput{
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
func (ct *ConsensusTester) AddSiacoinInputToTransaction(inputT Transaction, sci SiacoinInput) (t Transaction) {
	// Check that the function is being used correctly
	if sci.UnlockConditions.UnlockHash() != ct.UnlockConditions.UnlockHash() {
		ct.Fatal("misuse of AddSiacoinInputToTransaction - unlock conditions do not match")
	}

	// Add the input to the transaction.
	t = inputT
	t.SiacoinInputs = append(t.SiacoinInputs, sci)

	// Sign the input in an insecure way.
	tsig := TransactionSignature{
		ParentID:       crypto.Hash(sci.ParentID),
		CoveredFields:  CoveredFields{},
		PublicKeyIndex: 0,
	}
	tsigIndex := len(t.Signatures)
	t.Signatures = append(t.Signatures, tsig)
	sigHash := t.SigHash(tsigIndex)
	encodedSig, err := crypto.SignHash(sigHash, ct.SecretKey)
	if err != nil {
		ct.Fatal(err)
	}
	t.Signatures[tsigIndex].Signature = Signature(encodedSig[:])

	return
}

// SiacoinOutputTransaction creates and funds a transaction that has a siacoin
// output, and returns that transaction.
func (ct *ConsensusTester) SiacoinOutputTransaction() (txn Transaction) {
	sci, value := ct.FindSpendableSiacoinInput()
	txn = ct.AddSiacoinInputToTransaction(Transaction{}, sci)
	txn.SiacoinOutputs = append(txn.SiacoinOutputs, SiacoinOutput{
		Value:      value,
		UnlockHash: ct.UnlockHash,
	})
	return
}

// FileContractTransaction creates and funds a transaction that has a file
// contract, and returns that transaction.
func (ct *ConsensusTester) FileContractTransaction(start BlockHeight, expiration BlockHeight) (txn Transaction, file []byte) {
	sci, value := ct.FindSpendableSiacoinInput()
	txn = ct.AddSiacoinInputToTransaction(Transaction{}, sci)

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
	txn.FileContracts = append(txn.FileContracts, FileContract{
		FileSize:       4e3,
		FileMerkleRoot: mRoot,
		Start:          start,
		Payout:         value,
		Expiration:     expiration,
		MissedProofOutputs: []SiacoinOutput{
			SiacoinOutput{
				Value: value,
			},
		},
		TerminationHash: ct.UnlockHash,
	})
	txn.FileContracts[0].ValidProofOutputs = []SiacoinOutput{SiacoinOutput{Value: value.Sub(txn.FileContracts[0].Tax())}}

	return
}
