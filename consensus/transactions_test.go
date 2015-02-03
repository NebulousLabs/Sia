package consensus

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
)

// signedOutputTxn funds itself by mining a block, and then uses the funds to
// create a signed output that is valid.
func signedOutputTxn(t *testing.T, s *State) (txn Transaction) {
	// Create the keys and a siacoin output that adds coins to the keys.
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		t.Fatal(err)
	}
	spendConditions := SpendConditions{
		NumSignatures: 1,
		PublicKeys: []SiaPublicKey{
			SiaPublicKey{
				Algorithm: ED25519Identifier,
				Key:       pk[:],
			},
		},
	}
	coinAddress := spendConditions.CoinAddress()
	minerPayouts := []SiacoinOutput{
		SiacoinOutput{
			Value:     CalculateCoinbase(s.height() + 1),
			SpendHash: coinAddress,
		},
	}

	// Mine the block that creates the output.
	b, err := mineTestingBlock(s.CurrentBlock().ID(), Timestamp(time.Now().Unix()), minerPayouts, nil, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	// Create the transaction that spends the output.
	input := SiacoinInput{
		OutputID:        b.MinerPayoutID(0),
		SpendConditions: spendConditions,
	}
	output := SiacoinOutput{
		Value:     CalculateCoinbase(s.height()),
		SpendHash: ZeroAddress,
	}
	txn = Transaction{
		SiacoinInputs:  []SiacoinInput{input},
		SiacoinOutputs: []SiacoinOutput{output},
	}

	// Sign the transaction.
	sig := TransactionSignature{
		InputID:        input.OutputID,
		CoveredFields:  CoveredFields{WholeTransaction: true},
		PublicKeyIndex: 0,
	}
	txn.Signatures = append(txn.Signatures, sig)
	sigHash := txn.SigHash(0)
	encodedSig, err := crypto.SignBytes(sigHash[:], sk)
	if err != nil {
		t.Fatal(err)
	}
	txn.Signatures[0].Signature = encodedSig[:]
	return
}

// testSingleOutput creates a block with one transaction that has inputs and
// outputs, and verifies that the output is accepted into the state.
func testSingleOutput(t *testing.T, s *State) {
	txn := signedOutputTxn(t, s)
	b, err := mineTestingBlock(s.CurrentBlock().ID(), Timestamp(time.Now().Unix()), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
}

// TestSingleOutput creates a new state and uses it to call testSingleOutput.
func TestSingleOutput(t *testing.T) {
	s := CreateGenesisState(Timestamp(time.Now().Unix()))
	testSingleOutput(t, s)
}
