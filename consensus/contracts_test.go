package consensus

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/hash"
)

// contractTxn funds and returns a transaction with a file contract.
func contractTxn(t *testing.T, s *State, delay BlockHeight, duration BlockHeight) (txn Transaction) {
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
	outputValue := CalculateCoinbase(s.height())
	err = outputValue.Sub(NewCurrency64(12e3))
	if err != nil {
		t.Fatal(err)
	}
	output := SiacoinOutput{
		Value:     outputValue,
		SpendHash: ZeroAddress,
	}
	successAddress := CoinAddress{1}
	failAddress := CoinAddress{2}
	contract := FileContract{
		FileSize:           4e3,
		Start:              s.height() + delay,
		End:                s.height() + delay + duration,
		Payout:             NewCurrency64(12e3),
		ValidProofAddress:  successAddress,
		MissedProofAddress: failAddress,
	}
	txn = Transaction{
		SiacoinInputs:  []SiacoinInput{input},
		SiacoinOutputs: []SiacoinOutput{output},
		FileContracts:  []FileContract{contract},
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

// storageProofTxn funds a contract, puts it in the state, and then returns a
// transaction with a storage proof for the contract.
func storageProofTxn(t *testing.T, s *State) (txn Transaction, cid ContractID) {
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

	// Create the file that the storage proof happens over.
	simpleFile := make([]byte, 4000)
	rand.Read(simpleFile)

	// Create the transaction that spends the output.
	input := SiacoinInput{
		OutputID:        b.MinerPayoutID(0),
		SpendConditions: spendConditions,
	}
	outputValue := CalculateCoinbase(s.height())
	err = outputValue.Sub(NewCurrency64(12e3))
	if err != nil {
		t.Fatal(err)
	}
	output := SiacoinOutput{
		Value:     outputValue,
		SpendHash: ZeroAddress,
	}
	merkleRoot, err := hash.BytesMerkleRoot(simpleFile)
	if err != nil {
		t.Fatal(err)
	}
	contract := FileContract{
		FileMerkleRoot: merkleRoot,
		FileSize:       4000,
		Start:          s.height() + 2,
		End:            s.height() + 2 + 25*1000,
		Payout:         NewCurrency64(12e3),
	}
	txn = Transaction{
		SiacoinInputs:  []SiacoinInput{input},
		SiacoinOutputs: []SiacoinOutput{output},
		FileContracts:  []FileContract{contract},
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

	// Put the transaction into a block.
	b, err = mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}

	// Create the transaction that has the storage proof.
	cid = txn.FileContractID(0)
	segmentIndex, err := s.StorageProofSegment(cid)
	if err != nil {
		t.Fatal(err)
	}
	numSegments := hash.CalculateSegments(4000)
	segment, hashes, err := hash.BuildReaderProof(bytes.NewReader(simpleFile), numSegments, segmentIndex)
	if err != nil {
		t.Fatal(err)
	}
	sp := StorageProof{
		ContractID: txn.FileContractID(0),
		Segment:    segment,
		HashSet:    hashes,
	}
	txn = Transaction{
		StorageProofs: []StorageProof{sp},
	}

	return
}

// testContractCreation adds a block with a file contract to the state and
// checks that the contract is accepted.
func testContractCreation(t *testing.T, s *State) {
	txn := contractTxn(t, s, 2, 25*1000)
	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}

	// Check that the contract made it into the state.
	_, exists := s.openContracts[txn.FileContractID(0)]
	if !exists {
		t.Error("file contract not found found in state after being created")
	}
}

// testMissedProof creates a contract but then doesn't submit the storage
// proof.
func testMissedProof(t *testing.T, s *State) {
	// Get the transaction with the contract that will not be fulfilled.
	txn := contractTxn(t, s, 2, 1)
	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}

	// Submit 2 blocks, which means the contract will be a missed proof.
	for i := 0; i < 2; i++ {
		b, err = mineValidBlock(s)
		if err != nil {
			t.Fatal(err)
		}
		err = s.AcceptBlock(b)
		if err != nil {
			t.Error(err)
		}
	}

	// Check that the contract was removed, and that the missed proof output
	// was added.
	cid := txn.FileContractID(0)
	_, exists := s.openContracts[cid]
	if exists {
		t.Error("file contract is still in state despite having terminated")
	}
	output, exists := s.unspentOutputs[cid.StorageProofOutputID(false)]
	if !exists {
		t.Error("missed storage proof output is not in state even though the proof was missed")
	}

	// Check that the money went to the right place.
	if output.SpendHash != txn.FileContracts[0].MissedProofAddress {
		t.Error("missed proof output sent to wrong address!")
	}
}

// testStorageProofSubmit adds a block with a valid storage proof to the state
// and checks that it is accepted, ending the contract supplying an output to
// the person.
func testStorageProofSubmit(t *testing.T, s *State) {
	txn, cid := storageProofTxn(t, s)

	// Get the contract for a later check. (Contract disappears after the block
	// is accepted).
	contract, exists := s.openContracts[cid]
	if !exists {
		t.Fatal("file contract doesn't exist in state")
	}

	b, err := mineTestingBlock(s.CurrentBlock().ID(), currentTime(), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}

	// Check that the storage proof made it into the state.
	_, exists = s.openContracts[cid]
	if exists {
		t.Error("file contract still in state even though a proof for it has been submitted")
	}
	output, exists := s.unspentOutputs[cid.StorageProofOutputID(true)]
	if !exists {
		t.Fatal("storage proof output not in state after storage proof was submitted")
	}

	// Check that the money went to the right place.
	if output.SpendHash != contract.ValidProofAddress {
		t.Error("money for valid proof was sent to wrong address")
	}
}

// TestContractCreation creates a new state and uses it to call
// testContractCreation.
func TestContractCreation(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testContractCreation(t, s)
}

// TestMissedProof creates a new state and uses it to call testMissedProof.
func TestMissedProof(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testMissedProof(t, s)
}

// TestStorageProofSubmit creates a new state and uses it to call
// testStorageProofSubmit.
func TestStorageProofSubmit(t *testing.T) {
	s := CreateGenesisState(currentTime())
	testStorageProofSubmit(t, s)
}
