package consensus

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// testSimpleBlock mines a simple block (no transactions except those
// automatically added by the miner) and adds it to the consnesus set.
func (cst *consensusSetTester) testSimpleBlock() {
	// Get the starting hash of the consenesus set.
	initialChecksum := cst.cs.dbConsensusChecksum()
	initialHeight := cst.cs.dbBlockHeight()
	initialBlockID := cst.cs.dbCurrentBlockID()

	// Mine and submit a block
	block, err := cst.miner.AddBlock()
	if err != nil {
		panic(err)
	}

	// Check that the consensus info functions changed as expected.
	resultingChecksum := cst.cs.dbConsensusChecksum()
	if initialChecksum == resultingChecksum {
		panic("checksum is unchanged after mining a block")
	}
	resultingHeight := cst.cs.dbBlockHeight()
	if resultingHeight != initialHeight+1 {
		panic("height of consensus set did not increase as expected")
	}
	currentPB := cst.cs.dbCurrentProcessedBlock()
	if currentPB.Block.ParentID != initialBlockID {
		panic("new processed block does not have correct information")
	}
	if currentPB.Block.ID() != block.ID() {
		panic("the state's current block is not reporting as the recently mined block.")
	}
	if currentPB.Height != initialHeight+1 {
		panic("the processed block is not reporting the correct height")
	}
	pathID, err := cst.cs.dbGetPath(currentPB.Height)
	if err != nil {
		panic(err)
	}
	if pathID != block.ID() {
		panic("current path does not point to the correct block")
	}

	// Revert the block that was just added to the consensus set and check for
	// parity with the original state of consensus.
	parent, err := cst.cs.dbGetBlockMap(currentPB.Block.ParentID)
	if err != nil {
		panic(err)
	}
	_, _, err = cst.cs.dbForkBlockchain(parent)
	if err != nil {
		panic(err)
	}
	if cst.cs.dbConsensusChecksum() != initialChecksum {
		panic("adding and reverting a block changed the consensus set")
	}
	// Re-add the block and check for parity with the first time it was added.
	// This test is useful because a different codepath is followed if the
	// diffs have already been generated.
	_, _, err = cst.cs.dbForkBlockchain(currentPB)
	if err != nil {
		panic(err)
	}
	if cst.cs.dbConsensusChecksum() != resultingChecksum {
		panic("adding, reverting, and reading a block was inconsistent with just adding the block")
	}
}

// TestIntegrationSimpleBlock creates a consensus set tester and uses it to
// call testSimpleBlock.
func TestIntegrationSimpleBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestIntegrationSimpleBlock")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	cst.testSimpleBlock()
}

// testSpendSiacoinsBlock mines a block with a transaction spending siacoins
// and adds it to the consensus set.
func (cst *consensusSetTester) testSpendSiacoinsBlock() {
	// Create a random destination address for the output in the transaction.
	destAddr := randAddress()

	// Create a block containing a transaction with a valid siacoin output.
	txnValue := types.NewCurrency64(1200)
	txnBuilder := cst.wallet.StartTransaction()
	err := txnBuilder.FundSiacoins(txnValue)
	if err != nil {
		panic(err)
	}
	outputIndex := txnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: txnValue, UnlockHash: destAddr})
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		panic(err)
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		panic(err)
	}

	// Mine and apply the block to the consensus set.
	_, err = cst.miner.AddBlock()
	if err != nil {
		panic(err)
	}

	// See that the destination output was created.
	outputID := txnSet[len(txnSet)-1].SiacoinOutputID(int(outputIndex))
	sco, err := cst.cs.dbGetSiacoinOutput(outputID)
	if err != nil {
		panic(err)
	}
	if sco.Value.Cmp(txnValue) != 0 {
		panic("output added with wrong value")
	}
	if sco.UnlockHash != destAddr {
		panic("output sent to the wrong address")
	}
}

// TestIntegrationSpendSiacoinsBlock creates a consensus set tester and uses it
// to call testSpendSiacoinsBlock.
func TestIntegrationSpendSiacoinsBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestSpendSiacoinsBlock")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	cst.testSpendSiacoinsBlock()
}

// testValidStorageProofBlocks adds a block with a file contract, and then
// submits a storage proof for that file contract.
func (cst *consensusSetTester) testValidStorageProofBlocks() {
	validProofDest := randAddress()

	// Create a file (as a bytes.Buffer) that will be used for the file
	// contract.
	filesize := uint64(4e3)
	file := randFile(filesize)
	merkleRoot, err := crypto.ReaderMerkleRoot(file)
	if err != nil {
		panic(err)
	}
	file.Seek(0, 0)

	// Create a file contract that will be successful.
	payout := types.NewCurrency64(400e6)
	fc := types.FileContract{
		FileSize:       filesize,
		FileMerkleRoot: merkleRoot,
		WindowStart:    cst.cs.dbBlockHeight() + 1,
		WindowEnd:      cst.cs.dbBlockHeight() + 2,
		Payout:         payout,
		ValidProofOutputs: []types.SiacoinOutput{{
			UnlockHash: validProofDest,
			Value:      types.PostTax(cst.cs.dbBlockHeight(), payout),
		}},
		MissedProofOutputs: []types.SiacoinOutput{{
			UnlockHash: types.UnlockHash{},
			Value:      types.PostTax(cst.cs.dbBlockHeight(), payout),
		}},
	}

	// Submit a transaction with the file contract.
	oldSiafundPool := cst.cs.dbGetSiafundPool()
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(payout)
	if err != nil {
		panic(err)
	}
	fcIndex := txnBuilder.AddFileContract(fc)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		panic(err)
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		panic(err)
	}
	_, err = cst.miner.AddBlock()
	if err != nil {
		panic(err)
	}

	// Check that the siafund pool was increased by the tax on the payout.
	siafundPool := cst.cs.dbGetSiafundPool()
	if siafundPool.Cmp(oldSiafundPool.Add(types.Tax(cst.cs.dbBlockHeight()-1, payout))) != 0 {
		panic("siafund pool was not increased correctly")
	}

	// Check that the file contract made it into the database.
	ti := len(txnSet) - 1
	fcid := txnSet[ti].FileContractID(int(fcIndex))
	_, err = cst.cs.dbGetFileContract(fcid)
	if err != nil {
		panic(err)
	}

	// Create and submit a storage proof for the file contract.
	segmentIndex, err := cst.cs.StorageProofSegment(fcid)
	if err != nil {
		panic(err)
	}
	segment, hashSet, err := crypto.BuildReaderProof(file, segmentIndex)
	if err != nil {
		panic(err)
	}
	sp := types.StorageProof{
		ParentID: fcid,
		HashSet:  hashSet,
	}
	copy(sp.Segment[:], segment)
	txnBuilder = cst.wallet.StartTransaction()
	txnBuilder.AddStorageProof(sp)
	txnSet, err = txnBuilder.Sign(true)
	if err != nil {
		panic(err)
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		panic(err)
	}
	_, err = cst.miner.AddBlock()
	if err != nil {
		panic(err)
	}

	// Check that the file contract has been removed.
	_, err = cst.cs.dbGetFileContract(fcid)
	if err != errNilItem {
		panic("file contract should not exist in the database")
	}

	// Check that the siafund pool has not changed.
	postProofPool := cst.cs.dbGetSiafundPool()
	if postProofPool.Cmp(siafundPool) != 0 {
		panic("siafund pool should not change after submitting a storage proof")
	}

	// Check that a delayed output was created for the valid proof.
	spoid := fcid.StorageProofOutputID(types.ProofValid, 0)
	dsco, err := cst.cs.dbGetDSCO(cst.cs.dbBlockHeight()+types.MaturityDelay, spoid)
	if err != nil {
		panic(err)
	}
	if dsco.UnlockHash != fc.ValidProofOutputs[0].UnlockHash {
		panic("wrong unlock hash in dsco")
	}
	if dsco.Value.Cmp(fc.ValidProofOutputs[0].Value) != 0 {
		panic("wrong sco value in dsco")
	}
}

// TestIntegrationValidStorageProofBlocks creates a consensus set tester and
// uses it to call testValidStorageProofBlocks.
func TestIntegrationValidStorageProofBlocks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestIntegrationValidStorageProofBlocks")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	cst.testValidStorageProofBlocks()
}

/// BREAK ///
/// BREAK ///
/// BREAK ///

// testFileContractsBlocks creates a series of blocks that create, revise,
// prove, and fail to prove file contracts.
func (cst *consensusSetTester) testFileContractsBlocks() error {
	var validProofDest, missedProofDest, revisionDest types.UnlockHash
	_, err := rand.Read(validProofDest[:])
	if err != nil {
		return err
	}
	_, err = rand.Read(missedProofDest[:])
	if err != nil {
		return err
	}
	_, err = rand.Read(revisionDest[:])
	if err != nil {
		return err
	}

	// Create a file (as a bytes.Buffer) that will be used for file contracts
	// and storage proofs.
	filesize := uint64(4e3)
	fileBytes := make([]byte, filesize)
	_, err = rand.Read(fileBytes)
	if err != nil {
		return err
	}
	file := bytes.NewReader(fileBytes)
	merkleRoot, err := crypto.ReaderMerkleRoot(file)
	if err != nil {
		return err
	}
	file.Seek(0, 0)

	// Create a file contract that will be successfully proven and an alternate
	// file contract which will be missed.
	payout := types.NewCurrency64(400e6)
	validFC := types.FileContract{
		FileSize:       filesize,
		FileMerkleRoot: merkleRoot,
		WindowStart:    cst.cs.height() + 2,
		WindowEnd:      cst.cs.height() + 4,
		Payout:         payout,
		ValidProofOutputs: []types.SiacoinOutput{{
			UnlockHash: validProofDest,
		}},
		MissedProofOutputs: []types.SiacoinOutput{{
			UnlockHash: missedProofDest,
		}},
		UnlockHash: types.UnlockConditions{}.UnlockHash(),
	}
	outputSize := payout.Sub(types.Tax(cst.cs.dbBlockHeight(), validFC.Payout))
	validFC.ValidProofOutputs[0].Value = outputSize
	validFC.MissedProofOutputs[0].Value = outputSize
	missedFC := types.FileContract{
		FileSize:       uint64(filesize),
		FileMerkleRoot: merkleRoot,
		WindowStart:    cst.cs.height() + 2,
		WindowEnd:      cst.cs.height() + 4,
		Payout:         payout,
		ValidProofOutputs: []types.SiacoinOutput{{
			Value:      outputSize,
			UnlockHash: validProofDest,
		}},
		MissedProofOutputs: []types.SiacoinOutput{{
			Value:      outputSize,
			UnlockHash: missedProofDest,
		}},
		UnlockHash: types.UnlockConditions{}.UnlockHash(),
	}

	// Create and fund a transaction with a file contract.
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(payout.Mul(types.NewCurrency64(2)))
	if err != nil {
		return err
	}
	validFCIndex := txnBuilder.AddFileContract(validFC)
	missedFCIndex := txnBuilder.AddFileContract(missedFC)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return err
	}
	ti := len(txnSet) - 1
	validFCID := txnSet[ti].FileContractID(int(validFCIndex))
	missedFCID := txnSet[ti].FileContractID(int(missedFCIndex))
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return err
	}
	_, err = cst.miner.AddBlock()
	if err != nil {
		return err
	}

	// Check that the siafund pool was increased.
	var siafundPool types.Currency
	err = cst.cs.db.Update(func(tx *bolt.Tx) error {
		siafundPool = getSiafundPool(tx)
		return nil
	})
	if err != nil {
		panic(err)
	}
	if siafundPool.Cmp(types.NewCurrency64(31200e3)) != 0 {
		return errors.New("siafund pool was not increased correctly")
	}

	// Submit a file contract revision to the missed-proof file contract.
	txn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          missedFCID,
			NewRevisionNumber: 1,

			NewFileSize:          10e3, // By changing the filesize without changing the hash, a proof should become impossible.
			NewFileMerkleRoot:    missedFC.FileMerkleRoot,
			NewWindowStart:       missedFC.WindowStart + 1,
			NewWindowEnd:         missedFC.WindowEnd,
			NewValidProofOutputs: missedFC.ValidProofOutputs,
			NewMissedProofOutputs: []types.SiacoinOutput{{
				Value:      outputSize,
				UnlockHash: revisionDest,
			}},
		}},
	}
	err = cst.tpool.AcceptTransactionSet([]types.Transaction{txn})
	if err != nil {
		return err
	}
	_, err = cst.miner.AddBlock()
	if err != nil {
		return err
	}

	// Check that the revision was successful.
	if cst.cs.db.getFileContracts(missedFCID).RevisionNumber != 1 {
		return errors.New("revision did not update revision number")
	}
	if cst.cs.db.getFileContracts(missedFCID).FileSize != 10e3 {
		return errors.New("revision did not update file contract size")
	}

	// Create a storage proof for the validFC and submit it in a block.
	spSegmentIndex, err := cst.cs.StorageProofSegment(validFCID)
	if err != nil {
		return err
	}
	segment, hashSet, err := crypto.BuildReaderProof(file, spSegmentIndex)
	if err != nil {
		return err
	}
	txn = types.Transaction{
		StorageProofs: []types.StorageProof{{
			ParentID: validFCID,
			HashSet:  hashSet,
		}},
	}
	copy(txn.StorageProofs[0].Segment[:], segment)
	err = cst.tpool.AcceptTransactionSet([]types.Transaction{txn})
	if err != nil {
		return err
	}
	_, err = cst.miner.AddBlock()
	lockID3 := cst.cs.mu.Lock()
	cst.cs.mu.Unlock(lockID3)

	if err != nil {
		return err
	}

	// Check that the valid contract was removed but the missed contract was
	// not.
	exists := cst.cs.db.inFileContracts(validFCID)
	if exists {
		return errors.New("valid file contract still exists in the consensus set")
	}
	exists = cst.cs.db.inFileContracts(missedFCID)
	if !exists {
		return errors.New("missed file contract was consumed by storage proof")
	}

	// Check that the file contract output made it into the set of delayed
	// outputs.
	validProofID := validFCID.StorageProofOutputID(types.ProofValid, 0)
	exists = cst.cs.db.inDelayedSiacoinOutputsHeight(cst.cs.height()+types.MaturityDelay, validProofID)
	if !exists {
		return errors.New("file contract payout is not in the delayed outputs set")
	}

	// Mine a block to close the window on the missed file contract.
	_, err = cst.miner.AddBlock()
	if err != nil {
		return err
	}
	exists = cst.cs.db.inFileContracts(validFCID)
	if exists {
		return errors.New("valid file contract still exists in the consensus set")
	}
	exists = cst.cs.db.inFileContracts(missedFCID)
	if exists {
		return errors.New("missed file contract was not consumed when the window was closed.")
	}

	// Mine enough blocks to get all of the outputs into the set of siacoin
	// outputs.
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err = cst.miner.AddBlock()
		if err != nil {
			return err
		}
	}

	// Check that all of the outputs have ended up at the right destination.
	if cst.cs.db.getSiacoinOutputs(validFCID.StorageProofOutputID(types.ProofValid, 0)).UnlockHash != validProofDest {
		return errors.New("file contract output did not end up at the right place.")
	}
	if cst.cs.db.getSiacoinOutputs(missedFCID.StorageProofOutputID(types.ProofMissed, 0)).UnlockHash != revisionDest {
		return errors.New("missed file proof output did not end up at the revised destination")
	}

	return nil
}

// TestFileContractsBlocks creates a consensus set tester and uses it to call
// testFileContractsBlocks.
func TestFileContractsBlocks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestFileContractsBlocks")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// COMPATv0.4.0
	//
	// Mine enough blocks to get above the file contract hardfork threshold
	// (10).
	for i := 0; i < 10; i++ {
		_, err = cst.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = cst.testFileContractsBlocks()
	if err != nil {
		t.Fatal(err)
	}
}
