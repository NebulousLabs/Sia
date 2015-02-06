package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
)

// signedOutputTxn funds itself by mining a block, and then uses the funds to
// create a signed output that is valid.
func signedOutputTxn(t *testing.T, s *State, algorithm Identifier) (txn Transaction) {
	// Create the keys and a siacoin output that adds coins to the keys.
	sk, pk, err := crypto.GenerateSignatureKeys()
	if err != nil {
		t.Fatal(err)
	}
	spendConditions := SpendConditions{
		NumSignatures: 1,
		PublicKeys: []SiaPublicKey{
			SiaPublicKey{
				Algorithm: algorithm,
				Key:       encoding.Marshal(pk),
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
	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), minerPayouts, nil, s.CurrentTarget())
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
	rawSig, err := crypto.SignBytes(sigHash[:], sk)
	if err != nil {
		t.Fatal(err)
	}
	txn.Signatures[0].Signature = encoding.Marshal(rawSig)
	return
}

// testForeignSignature adds a transaction that is signed by an unrecogmized
// identifier. This should be considered a valid signature by consensus.
func testForeignSignature(t *testing.T, s *State) {
	// Grab a transaction, create an invalid signature, but then change the
	// algorithm to an unknown algorithm. This should not trigger an error.
	nonAlgorithm := ED25519Identifier
	nonAlgorithm[0] = ' '
	txn := signedOutputTxn(t, s, nonAlgorithm)
	txn.Signatures[0].Signature[0]++
	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	// Check that that the output made it into the state.
	_, exists := s.unspentOutputs[txn.SiacoinOutputID(0)]
	if !exists {
		t.Error("single output did not make it into the state unspent outputs list")
	}
}

// testInvalidSignature submits a transaction with a falsified signature.
func testInvalidSignature(t *testing.T, s *State) {
	txn := signedOutputTxn(t, s, ED25519Identifier)
	txn.Signatures[0].Signature[1]++
	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != InvalidSignatureErr {
		t.Fatal(err)
	}
}

// testSingleOutput creates a block with one transaction that has inputs and
// outputs, and verifies that the output is accepted into the state.
func testSingleOutput(t *testing.T, s *State) {
	txn := signedOutputTxn(t, s, ED25519Identifier)
	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}

	// Check that that the output made it into the state.
	_, exists := s.unspentOutputs[txn.SiacoinOutputID(0)]
	if !exists {
		t.Error("single output did not make it into the state unspent outputs list")
	}
}

// testUnsignedTransaction creates a valid transaction but then removes the
// signature.
func testUnsignedTransaction(t *testing.T, s *State) {
	txn := signedOutputTxn(t, s, ED25519Identifier)
	txn.Signatures = nil
	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != MissingSignaturesErr {
		t.Fatal(err)
	}
}

// TestForeignSignature creates a new state and uses it to call
// testForeignSignature.
func TestForeignSignature(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testForeignSignature(t, s)
}

// TestInvalidSignature creates a new state and uses it to call
// testInvalidSignature.
func TestInvalidSignature(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testInvalidSignature(t, s)
}

// TestSingleOutput creates a new state and uses it to call testSingleOutput.
func TestSingleOutput(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testSingleOutput(t, s)
}

// TestUnsignedTransaction creates a new state and uses it to call
// testUnsignedTransaction.
func TestUnsignedTransaction(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testUnsignedTransaction(t, s)
}
