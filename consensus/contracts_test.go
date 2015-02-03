package consensus

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
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
		Value:     CalculateCoinbase(s.height()) - 12*1000,
		SpendHash: ZeroAddress,
	}
	contract := FileContract{
		FileSize: 4000,
		Start:    s.height() + delay,
		End:      s.height() + delay + duration,
		Payout:   12 * 1000,
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
	encodedSig, err := crypto.SignBytes(sigHash[:], sk)
	if err != nil {
		t.Fatal(err)
	}
	txn.Signatures[0].Signature = encodedSig[:]
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

	// Create the file that the storage proof happens over.
	simpleFile := make([]byte, 4000)
	rand.Read(simpleFile)

	// Create the transaction that spends the output.
	input := SiacoinInput{
		OutputID:        b.MinerPayoutID(0),
		SpendConditions: spendConditions,
	}
	output := SiacoinOutput{
		Value:     CalculateCoinbase(s.height()) - 12*1000,
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
		Payout:         12 * 1000,
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
	encodedSig, err := crypto.SignBytes(sigHash[:], sk)
	if err != nil {
		t.Fatal(err)
	}
	txn.Signatures[0].Signature = encodedSig[:]

	// Put the transaction into a block.
	b, err = mineTestingBlock(s.CurrentBlock().ID(), Timestamp(time.Now().Unix()), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
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
	b, err := mineTestingBlock(s.CurrentBlock().ID(), Timestamp(time.Now().Unix()), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
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

// testStorageProofSubmit adds a block with a valid storage proof to the state
// and checks that it is accepted, ending the contract supplying an output to
// the person.
func testStorageProofSubmit(t *testing.T, s *State) {
	txn, cid := storageProofTxn(t, s)
	b, err := mineTestingBlock(s.CurrentBlock().ID(), Timestamp(time.Now().Unix()), nullMinerPayouts(s.Height()+1), []Transaction{txn}, s.CurrentTarget())
	if err != nil {
		t.Fatal(err)
	}
	err = s.AcceptBlock(b)
	if err != nil {
		t.Error(err)
	}

	// Check that the storage proof made it into the state.
	_, exists := s.openContracts[cid]
	if exists {
		t.Error("file contract still in state even though a proof for it has been submitted")
	}
	_, exists = s.unspentOutputs[cid.StorageProofOutputID(true)]
	if !exists {
		t.Error("storage proof output not in state after storage proof was submitted")
	}
}

// TestContractCreation creates a new state and uses it to call
// testContractCreation.
func TestContractCreation(t *testing.T) {
	s := CreateGenesisState(Timestamp(time.Now().Unix()))
	testContractCreation(t, s)
}

// TestStorageProofSubmit creates a new state and uses it to call
// testStorageProofSubmit.
func TestStorageProofSubmit(t *testing.T) {
	s := CreateGenesisState(Timestamp(time.Now().Unix()))
	testStorageProofSubmit(t, s)
}
