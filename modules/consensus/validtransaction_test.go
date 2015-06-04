package consensus

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// TestStorageProofSegment probes the storageProofSegment method of the
// consensus set.
func TestStorageProofSegment(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestStorageProofSegment")
	if err != nil {
		t.Fatal(err)
	}

	// Add a file contract to the consensus set that can be used to probe the
	// storage segment.
	var outputs []byte
	for i := 0; i < 4*256*256; i++ {
		var fcid types.FileContractID
		rand.Read(fcid[:])
		fc := types.FileContract{
			WindowStart: 2,
			FileSize:    256 * 64,
		}
		cst.cs.fileContracts[fcid] = fc
		index, err := cst.cs.storageProofSegment(fcid)
		if err != nil {
			t.Error(err)
		}
		outputs = append(outputs, byte(index))
	}

	// Perform entropy testing on 'outputs' to verify randomness.
	var b bytes.Buffer
	zip := gzip.NewWriter(&b)
	_, err = zip.Write(outputs)
	if err != nil {
		t.Fatal(err)
	}
	zip.Close()
	if b.Len() < len(outputs) {
		t.Error("supposedly high entropy random segments have been compressed!")
	}
}

// TestValidStorageProofs probes the validStorageProofs method of the consensus
// set.
func TestValidStorageProofs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestValidStorageProofs")
	if err != nil {
		t.Fatal(err)
	}

	// Create a file contract for which a storage proof can be created.
	var fcid types.FileContractID
	fcid[0] = 12
	simFile := make([]byte, 64*1024)
	rand.Read(simFile)
	buffer := bytes.NewReader(simFile)
	root, err := crypto.ReaderMerkleRoot(buffer)
	if err != nil {
		t.Fatal(err)
	}
	fc := types.FileContract{
		FileSize:       64 * 1024,
		FileMerkleRoot: root,
		WindowStart:    2,
		WindowEnd:      1200,
	}
	cst.cs.fileContracts[fcid] = fc
	buffer.Seek(0, 0)

	// Create a transaction with a storage proof.
	proofIndex, err := cst.cs.storageProofSegment(fcid)
	if err != nil {
		t.Fatal(err)
	}
	base, proofSet, err := crypto.BuildReaderProof(buffer, proofIndex)
	if err != nil {
		t.Fatal(err)
	}
	txn := types.Transaction{
		StorageProofs: []types.StorageProof{
			{
				ParentID: fcid,
				Segment:  base,
				HashSet:  proofSet,
			},
		},
	}
	err = cst.cs.validStorageProofs(txn)
	if err != nil {
		t.Error(err)
	}

	// Corrupt the proof set.
	proofSet[0][0]++
	txn = types.Transaction{
		StorageProofs: []types.StorageProof{
			{
				ParentID: fcid,
				Segment:  base,
				HashSet:  proofSet,
			},
		},
	}
	err = cst.cs.validStorageProofs(txn)
	if err != ErrInvalidStorageProof {
		t.Error(err)
	}
}

// TestValidFileContractRevisions probes the validFileContractRevisions method
// of the consensus set.
func TestValidFileContractRevisions(t *testing.T) {
	if testing.Short() {
		// t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestValidStorageProofs")
	if err != nil {
		t.Fatal(err)
	}

	// Grab an address + unlock conditions for the transaction.
	unlockHash, unlockConditions, err := cst.wallet.CoinAddress(false)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file contract for which a storage proof can be created.
	var fcid types.FileContractID
	fcid[0] = 12
	simFile := make([]byte, 64*1024)
	rand.Read(simFile)
	buffer := bytes.NewReader(simFile)
	root, err := crypto.ReaderMerkleRoot(buffer)
	if err != nil {
		t.Fatal(err)
	}
	fc := types.FileContract{
		FileSize:       64 * 1024,
		FileMerkleRoot: root,
		WindowStart:    102,
		WindowEnd:      1200,
		UnlockHash:     unlockHash,
		RevisionNumber: 1,
	}
	cst.cs.fileContracts[fcid] = fc

	// Try a working file contract revision.
	txn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{
			{
				ParentID:          fcid,
				UnlockConditions:  unlockConditions,
				NewRevisionNumber: 2,
			},
		},
	}
	err = cst.cs.validFileContractRevisions(txn)
	if err != nil {
		t.Error(err)
	}

	// Try a transaction with an insufficient revision number.
	txn = types.Transaction{
		FileContractRevisions: []types.FileContractRevision{
			{
				ParentID:          fcid,
				UnlockConditions:  unlockConditions,
				NewRevisionNumber: 1,
			},
		},
	}
	err = cst.cs.validFileContractRevisions(txn)
	if err != ErrLowRevisionNumber {
		t.Error(err)
	}
	txn = types.Transaction{
		FileContractRevisions: []types.FileContractRevision{
			{
				ParentID:          fcid,
				UnlockConditions:  unlockConditions,
				NewRevisionNumber: 0,
			},
		},
	}
	err = cst.cs.validFileContractRevisions(txn)
	if err != ErrLowRevisionNumber {
		t.Error(err)
	}
}
