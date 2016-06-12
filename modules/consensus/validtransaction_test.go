package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// TestTryValidTransactionSet submits a valid transaction set to the
// TryTransactionSet method.
func TestTryValidTransactionSet(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestTryValidTransactionSet")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	initialHash := cst.cs.dbConsensusChecksum()

	// Try a valid transaction.
	_, err = cst.wallet.SendSiacoins(types.NewCurrency64(1), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	txns := cst.tpool.TransactionList()
	cc, err := cst.cs.TryTransactionSet(txns)
	if err != nil {
		t.Error(err)
	}
	if cst.cs.dbConsensusChecksum() != initialHash {
		t.Error("TryTransactionSet did not resotre order")
	}
	if len(cc.SiacoinOutputDiffs) == 0 {
		t.Error("consensus change is missing diffs after verifying a transction clump")
	}
}

// TestTryInvalidTransactionSet submits an invalid transaction set to the
// TryTransaction method.
func TestTryInvalidTransactionSet(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestTryInvalidTransactionSet")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	initialHash := cst.cs.dbConsensusChecksum()

	// Try a valid transaction followed by an invalid transaction.
	_, err = cst.wallet.SendSiacoins(types.NewCurrency64(1), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	txns := cst.tpool.TransactionList()
	txn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{}},
	}
	txns = append(txns, txn)
	cc, err := cst.cs.TryTransactionSet(txns)
	if err == nil {
		t.Error("bad transaction survived filter")
	}
	if cst.cs.dbConsensusChecksum() != initialHash {
		t.Error("TryTransactionSet did not restore order")
	}
	if len(cc.SiacoinOutputDiffs) != 0 {
		t.Error("consensus change was not empty despite an error being returned")
	}
}

// TestStorageProofBoundaries creates file contracts and submits storage proofs
// for them, probing segment boundaries (first segment, last segment,
// incomplete segment, etc.).
func TestStorageProofBoundaries(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestStorageProofBoundaries")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	// Mine enough blocks to put us beyond the testing hardfork.
	for i := 0; i < 10; i++ {
		_, err = cst.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Try storage proofs on data between 0 bytes and 128 bytes (0 segments and
	// 1 segment). Perform the operation five times because we can't control
	// which segment gets selected - it is randomly decided by the block.
	segmentRange := []int{0, 1, 2, 3, 4, 5, 15, 25, 30, 32, 62, 63, 64, 65, 66, 70, 81, 89, 90, 126, 127, 128, 129}
	for i := 0; i < 3; i++ {
		randData, err := crypto.RandBytes(140)
		if err != nil {
			t.Fatal(err)
		}

		// Create a file contract for all sizes of the data between 0 and 2
		// segments and put them in the transaction pool.
		var fcids []types.FileContractID
		for _, k := range segmentRange {
			// Create the data and the file contract around it.
			truncatedData := randData[:k]
			fc := types.FileContract{
				FileSize:           uint64(k),
				FileMerkleRoot:     crypto.MerkleRoot(truncatedData),
				WindowStart:        cst.cs.dbBlockHeight() + 2,
				WindowEnd:          cst.cs.dbBlockHeight() + 4,
				Payout:             types.NewCurrency64(500), // Too small to be subject to siafund fee.
				ValidProofOutputs:  []types.SiacoinOutput{{Value: types.NewCurrency64(500)}},
				MissedProofOutputs: []types.SiacoinOutput{{Value: types.NewCurrency64(500)}},
			}

			// Create a transaction around the file contract and add it to the
			// transaction pool.
			b := cst.wallet.StartTransaction()
			err = b.FundSiacoins(types.NewCurrency64(500))
			if err != nil {
				t.Fatal(err)
			}
			b.AddFileContract(fc)
			txnSet, err := b.Sign(true)
			if err != nil {
				t.Fatal(err)
			}
			err = cst.tpool.AcceptTransactionSet(txnSet)
			if err != nil {
				t.Fatal(err)
			}

			// Store the file contract id for later when building the storage
			// proof.
			fcids = append(fcids, txnSet[len(txnSet)-1].FileContractID(0))
		}

		// Mine blocks to get the file contracts into the blockchain and
		// confirming.
		for j := 0; j < 2; j++ {
			_, err = cst.miner.AddBlock()
			if err != nil {
				t.Fatal(err)
			}
		}

		// Create storage proofs for the file contracts and submit the proofs
		// to the blockchain.
		for j, k := range segmentRange {
			// Build the storage proof.
			truncatedData := randData[:k]
			proofIndex, err := cst.cs.StorageProofSegment(fcids[j])
			if err != nil {
				t.Fatal(err)
			}
			base, hashSet := crypto.MerkleProof(truncatedData, proofIndex)
			sp := types.StorageProof{
				ParentID: fcids[j],
				HashSet:  hashSet,
			}
			copy(sp.Segment[:], base)

			if k > 0 {
				// Try submitting an empty storage proof, to make sure that the
				// hardfork code didn't accidentally allow empty storage proofs
				// in situations other than file sizes with 0 bytes.
				badSP := types.StorageProof{ParentID: fcids[j]}
				badTxn := types.Transaction{
					StorageProofs: []types.StorageProof{badSP},
				}
				err = cst.tpool.AcceptTransactionSet([]types.Transaction{badTxn})
				if err == nil {
					t.Fatal("bad storage proof got accepted into the transaction pool")
				}
			}

			// Submit the storage proof to the blockchain in a transaction.
			txn := types.Transaction{
				StorageProofs: []types.StorageProof{sp},
			}
			err = cst.tpool.AcceptTransactionSet([]types.Transaction{txn})
			if err != nil {
				t.Fatal(err, "-", j, k)
			}
		}

		// Mine blocks to get the storage proofs on the blockchain.
		for j := 0; j < 2; j++ {
			_, err = cst.miner.AddBlock()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestEmptyStorageProof creates file contracts and submits storage proofs for
// them, probing segment boundaries (first segment, last segment, incomplete
// segment, etc.).
func TestEmptyStorageProof(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestStorageProofBoundaries")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	// Mine enough blocks to put us beyond the testing hardfork.
	for i := 0; i < 10; i++ {
		_, err = cst.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Try storage proofs on data between 0 bytes and 128 bytes (0 segments and
	// 1 segment). Perform the operation five times because we can't control
	// which segment gets selected - it is randomly decided by the block.
	segmentRange := []int{0, 1, 2, 3, 4, 5, 15, 25, 30, 32, 62, 63, 64, 65, 66, 70, 81, 89, 90, 126, 127, 128, 129}
	for i := 0; i < 3; i++ {
		randData, err := crypto.RandBytes(140)
		if err != nil {
			t.Fatal(err)
		}

		// Create a file contract for all sizes of the data between 0 and 2
		// segments and put them in the transaction pool.
		var fcids []types.FileContractID
		for _, k := range segmentRange {
			// Create the data and the file contract around it.
			truncatedData := randData[:k]
			fc := types.FileContract{
				FileSize:           uint64(k),
				FileMerkleRoot:     crypto.MerkleRoot(truncatedData),
				WindowStart:        cst.cs.dbBlockHeight() + 2,
				WindowEnd:          cst.cs.dbBlockHeight() + 4,
				Payout:             types.NewCurrency64(500), // Too small to be subject to siafund fee.
				ValidProofOutputs:  []types.SiacoinOutput{{Value: types.NewCurrency64(500)}},
				MissedProofOutputs: []types.SiacoinOutput{{Value: types.NewCurrency64(500)}},
			}

			// Create a transaction around the file contract and add it to the
			// transaction pool.
			b := cst.wallet.StartTransaction()
			err = b.FundSiacoins(types.NewCurrency64(500))
			if err != nil {
				t.Fatal(err)
			}
			b.AddFileContract(fc)
			txnSet, err := b.Sign(true)
			if err != nil {
				t.Fatal(err)
			}
			err = cst.tpool.AcceptTransactionSet(txnSet)
			if err != nil {
				t.Fatal(err)
			}

			// Store the file contract id for later when building the storage
			// proof.
			fcids = append(fcids, txnSet[len(txnSet)-1].FileContractID(0))
		}

		// Mine blocks to get the file contracts into the blockchain and
		// confirming.
		for j := 0; j < 2; j++ {
			_, err = cst.miner.AddBlock()
			if err != nil {
				t.Fatal(err)
			}
		}

		// Create storage proofs for the file contracts and submit the proofs
		// to the blockchain.
		for j, k := range segmentRange {
			// Build the storage proof.
			truncatedData := randData[:k]
			proofIndex, err := cst.cs.StorageProofSegment(fcids[j])
			if err != nil {
				t.Fatal(err)
			}
			base, hashSet := crypto.MerkleProof(truncatedData, proofIndex)
			sp := types.StorageProof{
				ParentID: fcids[j],
				HashSet:  hashSet,
			}
			copy(sp.Segment[:], base)

			// Submit the storage proof to the blockchain in a transaction.
			txn := types.Transaction{
				StorageProofs: []types.StorageProof{sp},
			}
			err = cst.tpool.AcceptTransactionSet([]types.Transaction{txn})
			if err != nil {
				t.Fatal(err, "-", j, k)
			}
		}

		// Mine blocks to get the storage proofs on the blockchain.
		for j := 0; j < 2; j++ {
			_, err = cst.miner.AddBlock()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

/*
// TestValidSiacoins probes the validSiacoins method of the consensus set.
func TestValidSiacoins(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestValidSiacoins")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a transaction pointing to a nonexistent siacoin output.
	txn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{}},
	}
	err = cst.cs.validSiacoins(txn)
	if err != ErrMissingSiacoinOutput {
		t.Error(err)
	}

	// Create a transaction with invalid unlock conditions.
	var scoid types.SiacoinOutputID
	cst.cs.db.forEachSiacoinOutputs(func(mapScoid types.SiacoinOutputID, sco types.SiacoinOutput) {
		scoid = mapScoid
	})
	txn = types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID: scoid,
		}},
	}
	err = cst.cs.validSiacoins(txn)
	if err != ErrWrongUnlockConditions {
		t.Error(err)
	}

	// Create a txn with more outputs than inputs.
	txn = types.Transaction{
		SiacoinOutputs: []types.SiacoinOutput{{
			Value: types.NewCurrency64(1),
		}},
	}
	err = cst.cs.validSiacoins(txn)
	if err != ErrSiacoinInputOutputMismatch {
		t.Error(err)
	}
}

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
	defer cst.closeCst()

	// Submit a file contract that is unrecognized.
	_, err = cst.cs.storageProofSegment(types.FileContractID{})
	if err != ErrUnrecognizedFileContractID {
		t.Error(err)
	}

	// Try to get the segment of an unfinished file contract.
	cst.cs.db.addFileContracts(types.FileContractID{}, types.FileContract{
		WindowStart: 100000,
	})
	_, err = cst.cs.storageProofSegment(types.FileContractID{})
	if err != ErrUnfinishedFileContract {
		t.Error(err)
	}
}

// TestStorageProofSegmentRandomness checks that the storageProofSegment method
// is producing outputs that pass an imperfect randomness check (gzip).
func TestStorageProofSegmentRandomness(t *testing.T) {
	t.Skip("randomness check takes a long time")
	cst, err := createConsensusSetTester("TestStorageProofSegmentRandomness")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Add a file contract to the consensus set that can be used to probe the
	// storage segment.
	var outputs []byte
	for i := 0; i < 32*256; i++ {
		var fcid types.FileContractID
		rand.Read(fcid[:])
		fc := types.FileContract{
			WindowStart: 2,
			FileSize:    256 * 64,
		}
		cst.cs.db.addFileContracts(fcid, fc)
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
	defer cst.closeCst()

	// COMPATv0.4.0
	//
	// Mine 10 blocks so that the post-hardfork rules are in effect.
	for i := 0; i < 10; i++ {
		block, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create a file contract for which a storage proof can be created.
	var fcid types.FileContractID
	fcid[0] = 12
	simFile := make([]byte, 64*1024)
	_, err = rand.Read(simFile)
	if err != nil {
		t.Fatal(err)
	}
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
	cst.cs.db.addFileContracts(fcid, fc)
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
		StorageProofs: []types.StorageProof{{
			ParentID: fcid,
			HashSet:  proofSet,
		}},
	}
	copy(txn.StorageProofs[0].Segment[:], base)
	err = cst.cs.validStorageProofs(txn)
	if err != nil {
		t.Error(err)
	}

	// Corrupt the proof set.
	proofSet[0][0]++
	txn = types.Transaction{
		StorageProofs: []types.StorageProof{{
			ParentID: fcid,
			HashSet:  proofSet,
		}},
	}
	copy(txn.StorageProofs[0].Segment[:], base)
	err = cst.cs.validStorageProofs(txn)
	if err != ErrInvalidStorageProof {
		t.Error(err)
	}

	// Try to validate a proof for a file contract that doesn't exist.
	txn.StorageProofs[0].ParentID = types.FileContractID{}
	err = cst.cs.validStorageProofs(txn)
	if err != ErrUnrecognizedFileContractID {
		t.Error(err)
	}

	// Try a proof set where there is padding on the last segment in the file.
	file := make([]byte, 100)
	_, err = rand.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	buffer = bytes.NewReader(file)
	root, err = crypto.ReaderMerkleRoot(buffer)
	if err != nil {
		t.Fatal(err)
	}
	fc = types.FileContract{
		FileSize:       100,
		FileMerkleRoot: root,
		WindowStart:    2,
		WindowEnd:      1200,
	}
	buffer.Seek(0, 0)

	// Find a proofIndex that has the value '1'.
	for {
		fcid[0]++
		cst.cs.db.addFileContracts(fcid, fc)
		proofIndex, err = cst.cs.storageProofSegment(fcid)
		if err != nil {
			t.Fatal(err)
		}
		if proofIndex == 1 {
			break
		}
	}
	base, proofSet, err = crypto.BuildReaderProof(buffer, proofIndex)
	if err != nil {
		t.Fatal(err)
	}
	txn = types.Transaction{
		StorageProofs: []types.StorageProof{{
			ParentID: fcid,
			HashSet:  proofSet,
		}},
	}
	copy(txn.StorageProofs[0].Segment[:], base)
	err = cst.cs.validStorageProofs(txn)
	if err != nil {
		t.Fatal(err)
	}
}

// COMPATv0.4.0
//
// TestPreForkValidStorageProofs checks that storage proofs which are invalid
// before the hardfork (but valid afterwards) are still rejected before the
// hardfork).
func TestPreForkValidStorageProofs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestPreForkValidStorageProofs")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Try a proof set where there is padding on the last segment in the file.
	file := make([]byte, 100)
	_, err = rand.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	buffer := bytes.NewReader(file)
	root, err := crypto.ReaderMerkleRoot(buffer)
	if err != nil {
		t.Fatal(err)
	}
	fc := types.FileContract{
		FileSize:       100,
		FileMerkleRoot: root,
		WindowStart:    2,
		WindowEnd:      1200,
	}
	buffer.Seek(0, 0)

	// Find a proofIndex that has the value '1'.
	var fcid types.FileContractID
	var proofIndex uint64
	for {
		fcid[0]++
		cst.cs.db.addFileContracts(fcid, fc)
		proofIndex, err = cst.cs.storageProofSegment(fcid)
		if err != nil {
			t.Fatal(err)
		}
		if proofIndex == 1 {
			break
		}
	}
	base, proofSet, err := crypto.BuildReaderProof(buffer, proofIndex)
	if err != nil {
		t.Fatal(err)
	}
	txn := types.Transaction{
		StorageProofs: []types.StorageProof{{
			ParentID: fcid,
			HashSet:  proofSet,
		}},
	}
	copy(txn.StorageProofs[0].Segment[:], base)
	err = cst.cs.validStorageProofs(txn)
	if err != ErrInvalidStorageProof {
		t.Fatal(err)
	}
}

// TestValidFileContractRevisions probes the validFileContractRevisions method
// of the consensus set.
func TestValidFileContractRevisions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestValidFileContractRevisions")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Grab an address + unlock conditions for the transaction.
	unlockConditions, err := cst.wallet.NextAddress()
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
		UnlockHash:     unlockConditions.UnlockHash(),
		RevisionNumber: 1,
	}
	cst.cs.db.addFileContracts(fcid, fc)

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

	// Submit a file contract revision pointing to an invalid parent.
	txn.FileContractRevisions[0].ParentID[0]--
	err = cst.cs.validFileContractRevisions(txn)
	if err != ErrUnrecognizedFileContractID {
		t.Error(err)
	}
	txn.FileContractRevisions[0].ParentID[0]++

	// Submit a file contract revision for a file contract whose window has
	// already opened.
	fc = cst.cs.db.getFileContracts(fcid)
	fc.WindowStart = 0
	cst.cs.db.rmFileContracts(fcid)
	cst.cs.db.addFileContracts(fcid, fc)
	txn.FileContractRevisions[0].NewRevisionNumber = 3
	err = cst.cs.validFileContractRevisions(txn)
	if err != ErrLateRevision {
		t.Error(err)
	}

	// Submit a file contract revision with incorrect unlock conditions.
	fc.WindowStart = 100
	cst.cs.db.rmFileContracts(fcid)
	cst.cs.db.addFileContracts(fcid, fc)
	txn.FileContractRevisions[0].UnlockConditions.Timelock++
	err = cst.cs.validFileContractRevisions(txn)
	if err != ErrWrongUnlockConditions {
		t.Error(err)
	}
	txn.FileContractRevisions[0].UnlockConditions.Timelock--

	// Submit file contract revisions for file contracts with altered payouts.
	txn.FileContractRevisions[0].NewValidProofOutputs = []types.SiacoinOutput{{
		Value: types.NewCurrency64(1),
	}}
	txn.FileContractRevisions[0].NewMissedProofOutputs = []types.SiacoinOutput{{
		Value: types.NewCurrency64(1),
	}}
	err = cst.cs.validFileContractRevisions(txn)
	if err != ErrAlteredRevisionPayouts {
		t.Error(err)
	}
	txn.FileContractRevisions[0].NewValidProofOutputs = nil
	err = cst.cs.validFileContractRevisions(txn)
	if err != ErrAlteredRevisionPayouts {
		t.Error(err)
	}
	txn.FileContractRevisions[0].NewValidProofOutputs = []types.SiacoinOutput{{
		Value: types.NewCurrency64(1),
	}}
	txn.FileContractRevisions[0].NewMissedProofOutputs = nil
	err = cst.cs.validFileContractRevisions(txn)
	if err != ErrAlteredRevisionPayouts {
		t.Error(err)
	}
}

// TestValidSiafunds probes the validSiafunds mthod of the consensus set.
func TestValidSiafunds(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestValidSiafunds")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a transaction pointing to a nonexistent siafund output.
	txn := types.Transaction{
		SiafundInputs: []types.SiafundInput{{}},
	}
	err = cst.cs.validSiafunds(txn)
	if err != ErrMissingSiafundOutput {
		t.Error(err)
	}

	// Create a transaction with invalid unlock conditions.
	var sfoid types.SiafundOutputID
	cst.cs.db.forEachSiafundOutputs(func(mapSfoid types.SiafundOutputID, sfo types.SiafundOutput) {
		sfoid = mapSfoid
		// pointless to do this but I can't think of a better way.
	})
	txn = types.Transaction{
		SiafundInputs: []types.SiafundInput{{
			ParentID:         sfoid,
			UnlockConditions: types.UnlockConditions{Timelock: 12345}, // avoid collisions with existing outputs
		}},
	}
	err = cst.cs.validSiafunds(txn)
	if err != ErrWrongUnlockConditions {
		t.Error(err)
	}

	// Create a transaction with more outputs than inputs.
	txn = types.Transaction{
		SiafundOutputs: []types.SiafundOutput{{
			Value: types.NewCurrency64(1),
		}},
	}
	err = cst.cs.validSiafunds(txn)
	if err != ErrSiafundInputOutputMismatch {
		t.Error(err)
	}
}

// TestValidTransaction probes the validTransaction method of the consensus
// set.
func TestValidTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestValidTransaction")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a transaction that is not standalone valid.
	txn := types.Transaction{
		FileContracts: []types.FileContract{{
			WindowStart: 0,
		}},
	}
	err = cst.cs.validTransaction(txn)
	if err == nil {
		t.Error("transaction is valid")
	}

	// Create a transaction with invalid siacoins.
	txn = types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{}},
	}
	err = cst.cs.validTransaction(txn)
	if err == nil {
		t.Error("transaction is valid")
	}

	// Create a transaction with invalid storage proofs.
	txn = types.Transaction{
		StorageProofs: []types.StorageProof{{}},
	}
	err = cst.cs.validTransaction(txn)
	if err == nil {
		t.Error("transaction is valid")
	}

	// Create a transaction with invalid file contract revisions.
	txn = types.Transaction{
		FileContractRevisions: []types.FileContractRevision{{
			NewWindowStart: 5000,
			NewWindowEnd:   5005,
			ParentID:       types.FileContractID{},
		}},
	}
	err = cst.cs.validTransaction(txn)
	if err == nil {
		t.Error("transaction is valid")
	}

	// Create a transaction with invalid siafunds.
	txn = types.Transaction{
		SiafundInputs: []types.SiafundInput{{}},
	}
	err = cst.cs.validTransaction(txn)
	if err == nil {
		t.Error("transaction is valid")
	}
}
*/
