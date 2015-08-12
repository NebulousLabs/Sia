package consensus

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestDoSBlockHandling checks that saved bad blocks are correctly ignored.
func TestDoSBlockHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestDoSBlockHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Mine a DoS block and submit it to the state, expect a normal error.
	// Create a transaction that is funded but the funds are never spent. This
	// transaction is invalid in a way that triggers the DoS block detection.
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(types.NewCurrency64(50))
	if err != nil {
		t.Fatal(err)
	}
	txnSet, err := txnBuilder.Sign(true) // true indicates that the whole transaction should be signed.
	if err != nil {
		t.Fatal(err)
	}

	// Get a block, insert the transaction, and submit the block.
	block, _, target := cst.miner.BlockForWork()
	block.Transactions = append(block.Transactions, txnSet...)
	dosBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.AcceptBlock(dosBlock)
	if err != ErrSiacoinInputOutputMismatch {
		t.Fatal("unexpected err: " + err.Error())
	}

	// Submit the same DoS block to the state again, expect ErrDoSBlock.
	err = cst.cs.AcceptBlock(dosBlock)
	if err != ErrDoSBlock {
		t.Fatal("unexpected err: " + err.Error())
	}
}

// testBlockKnownHandling submits known blocks to the consensus set.
func (cst *consensusSetTester) testBlockKnownHandling() error {
	// Get a block destined to be stale.
	block, _, target := cst.miner.BlockForWork()
	staleBlock, _ := cst.miner.SolveBlock(block, target)

	// Add two new blocks to the consensus set to block the stale block.
	block1, _ := cst.miner.FindBlock()
	err := cst.cs.AcceptBlock(block1)
	if err != nil {
		return err
	}
	block2, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block2)
	if err != nil {
		return err
	}

	// Submit the stale block.
	err = cst.cs.acceptBlock(staleBlock)
	if err != nil && err != modules.ErrNonExtendingBlock {
		return err
	}

	// Submit block1 and block2 again, looking for a 'BlockKnown' error.
	err = cst.cs.acceptBlock(block1)
	if err != ErrBlockKnown {
		return errors.New("expecting known block err: " + err.Error())
	}
	err = cst.cs.acceptBlock(block2)
	if err != ErrBlockKnown {
		return errors.New("expecting known block err: " + err.Error())
	}
	err = cst.cs.acceptBlock(staleBlock)
	if err != ErrBlockKnown {
		return errors.New("expecting known block err: " + err.Error())
	}

	// Try the genesis block edge case.
	genesisBlock := cst.cs.db.getBlockMap(cst.cs.db.getPath(0)).Block
	err = cst.cs.acceptBlock(genesisBlock)
	if err != ErrBlockKnown {
		return errors.New("expecting known block err: " + err.Error())
	}
	return nil
}

// TestBlockKnownHandling creates a new consensus set tester and uses it to
// call testBlockKnownHandling.
func TestBlockKnownHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	cst, err := createConsensusSetTester("TestBlockKnownHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	err = cst.testBlockKnownHandling()
	if err != nil {
		t.Error(err)
	}
}

// TestOrphanHandling passes an orphan block to the consensus set.
func TestOrphanHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestOrphanHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// The empty block is an orphan.
	orphan := types.Block{}
	err = cst.cs.acceptBlock(orphan)
	if err != ErrOrphan {
		t.Error("expecting ErrOrphan:", err)
	}
	err = cst.cs.acceptBlock(orphan)
	if err != ErrOrphan {
		t.Error("expecting ErrOrphan:", err)
	}
}

// TestMissedTarget submits a block that does not meet the required target.
func TestMissedTarget(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMissedTarget")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Mine a block that doesn't meet the target.
	block, _, target := cst.miner.BlockForWork()
	for block.CheckTarget(target) && block.Nonce[0] != 255 {
		block.Nonce[0]++
	}
	if block.CheckTarget(target) {
		t.Fatal("unable to find a failing target (lol)")
	}
	err = cst.cs.acceptBlock(block)
	if err != ErrMissedTarget {
		t.Error("expecting ErrMissedTarget:", err)
	}
}

// testLargeBlock creates a block that is too large to be accepted by the state
// and checks that it actually gets rejected.
func TestLargeBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestLargeBlock")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a transaction that puts the block over the size limit.
	bigData := make([]byte, types.BlockSizeLimit)
	txn := types.Transaction{
		ArbitraryData: [][]byte{bigData},
	}

	// Fetch a block and add the transaction, then submit the block.
	block, _, target := cst.miner.BlockForWork()
	block.Transactions = append(block.Transactions, txn)
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.acceptBlock(solvedBlock)
	if err != ErrLargeBlock {
		t.Error(err)
	}
}

// TestEarlyBlockTimestampHandling checks that blocks with early timestamps are
// handled appropriately.
func TestEarlyBlockTimestampHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestBlockTimestampHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block with a too early timestamp - block should be rejected
	// outright.
	block, _, target := cst.miner.BlockForWork()
	earliestTimestamp := cst.cs.earliestChildTimestamp(cst.cs.db.getBlockMap(block.ParentID))
	block.Timestamp = earliestTimestamp - 1
	earlyBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.acceptBlock(earlyBlock)
	if err != ErrEarlyTimestamp {
		t.Error("expecting ErrEarlyTimestamp:", err.Error())
	}
}

// TestExtremeFutureTimestampHandling checks that blocks with extreme future
// timestamps handled correclty.
func TestExtremeFutureTimestampHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestExtremeFutureTimestampHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Submit a block with a timestamp in the extreme future.
	block, _, target := cst.miner.BlockForWork()
	block.Timestamp = types.CurrentTimestamp() + 2 + types.ExtremeFutureThreshold
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.acceptBlock(solvedBlock)
	if err != ErrExtremeFutureTimestamp {
		t.Error("Expecting ErrExtremeFutureTimestamp", err)
	}

	// Check that after waiting until the block is no longer in the future, the
	// block still has not been added to the consensus set (prove that the
	// block was correctly discarded).
	time.Sleep(time.Second * time.Duration(3+types.ExtremeFutureThreshold))
	lockID := cst.cs.mu.RLock()
	defer cst.cs.mu.RUnlock(lockID)
	exists := cst.cs.db.inBlockMap(solvedBlock.ID())
	if exists {
		t.Error("extreme future block made it into the consensus set after waiting")
	}
}

// TestMinerPayoutHandling checks that blocks with incorrect payouts are
// rejected.
func TestMinerPayoutHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestMinerPayoutHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a block with the wrong miner payout structure - testing can be
	// light here because there is heavier testing in the 'types' package,
	// where the logic is defined.
	block, _, target := cst.miner.BlockForWork()
	block.MinerPayouts = append(block.MinerPayouts, types.SiacoinOutput{Value: types.NewCurrency64(1)})
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.acceptBlock(solvedBlock)
	if err != ErrBadMinerPayouts {
		t.Error(err)
	}
}

// testFutureTimestampHandling checks that blocks in the future (but not
// extreme future) are handled correctly.
func (cst *consensusSetTester) testFutureTimestampHandling() error {
	// Submit a block with a timestamp in the future, but not the extreme
	// future.
	block, _, target := cst.miner.BlockForWork()
	block.Timestamp = types.CurrentTimestamp() + 2 + types.FutureThreshold
	solvedBlock, _ := cst.miner.SolveBlock(block, target)
	err := cst.cs.acceptBlock(solvedBlock)
	if err != ErrFutureTimestamp {
		return errors.New("Expecting ErrExtremeFutureTimestamp: " + err.Error())
	}

	// Check that after waiting until the block is no longer too far in the
	// future, the block gets added to the consensus set.
	time.Sleep(time.Second * 3) // 3 seconds, as the block was originally 2 seconds too far into the future.
	lockID := cst.cs.mu.RLock()
	defer cst.cs.mu.RUnlock(lockID)
	exists := cst.cs.db.inBlockMap(solvedBlock.ID())
	if !exists {
		return errors.New("future block was not added to the consensus set after waiting the appropriate amount of time.")
	}
	return nil
}

// TestFutureTimestampHandling creates a consensus set tester and uses it to
// call testFutureTimestampHandling.
func TestFutureTimestampHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestFutureTimestampHandling")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	err = cst.testFutureTimestampHandling()
	if err != nil {
		t.Error(err)
	}
}

// TestInconsistentCheck submits a block on a consensus set that is
// inconsistent, attempting to trigger a panic.
func TestInconsistentCheck(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestInconsistentCheck")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Corrupt the consensus set.
	var sfod types.SiafundOutputID
	var sfo types.SiafundOutput
	cst.cs.db.forEachSiafundOutputs(func(id types.SiafundOutputID, output types.SiafundOutput) {
		sfod = id
		sfo = output
	})
	sfo.Value = sfo.Value.Add(types.NewCurrency64(1))
	cst.cs.db.rmSiafundOutputs(sfod)
	cst.cs.db.addSiafundOutputs(sfod, sfo)

	// Mine and submit a block, triggering the inconsistency check.
	defer func() {
		r := recover()
		if r != errSiafundMiscount {
			t.Error("expecting errSiacoinMiscount, got:", r)
		}
	}()
	block, _ := cst.miner.FindBlock()
	_ = cst.cs.AcceptBlock(block)
}

// testSimpleBlock mines a simple block (no transactions except those
// automatically added by the miner) and adds it to the consnesus set.
func (cst *consensusSetTester) testSimpleBlock() error {
	// Get the starting hash of the consenesus set.
	initialCSSum := cst.cs.consensusSetHash()

	// Mine and submit a block
	block, _ := cst.miner.FindBlock()
	err := cst.cs.AcceptBlock(block)
	if err != nil {
		return err
	}

	// Get the ending hash of the consensus set.
	resultingCSSum := cst.cs.consensusSetHash()
	if initialCSSum == resultingCSSum {
		return errors.New("state hash is unchanged after mining a block")
	}

	// Check that the current path has updated as expected.
	newNode := cst.cs.currentProcessedBlock()
	if cst.cs.CurrentBlock().ID() != block.ID() {
		return errors.New("the state's current block is not reporting as the recently mined block.")
	}
	// Check that the current path has updated correctly.
	if block.ID() != cst.cs.db.getPath(newNode.Height) {
		return errors.New("the state's current path didn't update correctly after accepting a new block")
	}

	// Revert the block that was just added to the consensus set and check for
	// parity with the original state of consensus.
	parent := cst.cs.db.getBlockMap(newNode.Parent)
	_, _, err = cst.cs.forkBlockchain(parent)
	if err != nil {
		return err
	}
	if cst.cs.consensusSetHash() != initialCSSum {
		return errors.New("adding and reverting a block changed the consensus set")
	}
	// Re-add the block and check for parity with the first time it was added.
	// This test is useful because a different codepath is followed if the
	// diffs have already been generated.
	_, _, err = cst.cs.forkBlockchain(newNode)
	if cst.cs.consensusSetHash() != resultingCSSum {
		return errors.New("adding, reverting, and reading a block was inconsistent with just adding the block")
	}
	return nil
}

// TestSimpleBlock creates a consensus set tester and uses it to call
// testSimpleBlock.
func TestSimpleBlock(t *testing.T) {
	cst, err := createConsensusSetTester("TestSimpleBlock")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	err = cst.testSimpleBlock()
	if err != nil {
		t.Error(err)
	}
}

// testSpendSiacoinsBlock mines a block with a transaction spending siacoins
// and adds it to the consensus set.
func (cst *consensusSetTester) testSpendSiacoinsBlock() error {
	// Create a random destination address for the output in the transaction.
	var destAddr types.UnlockHash
	_, err := rand.Read(destAddr[:])
	if err != nil {
		return err
	}

	// Create a block containing a transaction with a valid siacoin output.
	txnValue := types.NewCurrency64(1200)
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(txnValue)
	if err != nil {
		return err
	}
	outputIndex := txnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: txnValue, UnlockHash: destAddr})
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return err
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return err
	}
	outputID := txnSet[len(txnSet)-1].SiacoinOutputID(int(outputIndex))

	// Mine and apply the block to the consensus set.
	block, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
	if err != nil {
		return err
	}

	// Find the destAddr among the outputs.
	var found bool
	cst.cs.db.forEachSiacoinOutputs(func(id types.SiacoinOutputID, output types.SiacoinOutput) {
		if id == outputID {
			if found {
				err = errors.New("output found twice")
			}
			if output.Value.Cmp(txnValue) != 0 {
				err = errors.New("output has wrong value")
			}
			found = true
		}
	})
	if err != nil {
		return err
	}
	if !found {
		return errors.New("could not find created siacoin output")
	}
	return nil
}

// TestSpendSiacoinsBlock creates a consensus set tester and uses it to call
// testSpendSiacoinsBlock.
func TestSpendSiacoinsBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestSpendSiacoinsBlock")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	err = cst.testSpendSiacoinsBlock()
	if err != nil {
		t.Error(err)
	}
}

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
	outputSize := payout.Sub(validFC.Tax())
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
	block, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
	if err != nil {
		return err
	}

	// Check that the siafund pool was increased.
	if cst.cs.siafundPool.Cmp(types.NewCurrency64(31200e3)) != 0 {
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
	block, _ = cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
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
	block, _ = cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
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
	_, exists = cst.cs.delayedSiacoinOutputs[cst.cs.height()+types.MaturityDelay][validProofID]
	if !exists {
		return errors.New("file contract payout is not in the delayed outputs set")
	}

	// Mine a block to close the window on the missed file contract.
	block, _ = cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
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
		block, _ = cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
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
		block, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}
	err = cst.testFileContractsBlocks()
	if err != nil {
		t.Fatal(err)
	}
}

// testSpendSiafundsBlock mines a block with a transaction spending siafunds
// and adds it to the consensus set.
func (cst *consensusSetTester) testSpendSiafundsBlock() error {
	// Create a destination for the siafunds.
	var destAddr types.UnlockHash
	_, err := rand.Read(destAddr[:])
	if err != nil {
		return err
	}

	// Find the siafund output that is 'anyone can spend' (output exists only
	// in the testing setup).
	var srcID types.SiafundOutputID
	var srcValue types.Currency
	anyoneSpends := types.UnlockConditions{}.UnlockHash()
	cst.cs.db.forEachSiafundOutputs(func(id types.SiafundOutputID, sfo types.SiafundOutput) {
		if sfo.UnlockHash == anyoneSpends {
			srcID = id
			srcValue = sfo.Value
		}
	})

	// Create a transaction that spends siafunds.
	txn := types.Transaction{
		SiafundInputs: []types.SiafundInput{{
			ParentID:         srcID,
			UnlockConditions: types.UnlockConditions{},
		}},
		SiafundOutputs: []types.SiafundOutput{
			{
				Value:      srcValue.Sub(types.NewCurrency64(1)),
				UnlockHash: types.UnlockConditions{}.UnlockHash(),
			},
			{
				Value:      types.NewCurrency64(1),
				UnlockHash: destAddr,
			},
		},
	}
	sfoid0 := txn.SiafundOutputID(0)
	sfoid1 := txn.SiafundOutputID(1)
	cst.tpool.AcceptTransactionSet([]types.Transaction{txn})

	// Mine a block containing the txn.
	block, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
	if err != nil {
		return err
	}

	// Check that the input got consumed, and that the outputs got created.
	exists := cst.cs.db.inSiafundOutputs(srcID)
	if exists {
		return errors.New("siafund output was not properly consumed")
	}
	exists = cst.cs.db.inSiafundOutputs(sfoid0)
	if !exists {
		return errors.New("siafund output was not properly created")
	}
	sfo := cst.cs.db.getSiafundOutputs(sfoid0)
	if sfo.Value.Cmp(srcValue.Sub(types.NewCurrency64(1))) != 0 {
		return errors.New("created siafund has wrong value")
	}
	if sfo.UnlockHash != anyoneSpends {
		return errors.New("siafund output sent to wrong unlock hash")
	}
	exists = cst.cs.db.inSiafundOutputs(sfoid1)
	if !exists {
		return errors.New("second siafund output was not properly created")
	}
	sfo = cst.cs.db.getSiafundOutputs(sfoid1)
	if sfo.Value.Cmp(types.NewCurrency64(1)) != 0 {
		return errors.New("second siafund output has wrong value")
	}
	if sfo.UnlockHash != destAddr {
		return errors.New("second siafund output sent to wrong addr")
	}

	// Put a file contract into the blockchain that will add values to siafund
	// outputs.
	oldSiafundPool := cst.cs.siafundPool
	payout := types.NewCurrency64(400e6)
	fc := types.FileContract{
		WindowStart: cst.cs.height() + 2,
		WindowEnd:   cst.cs.height() + 4,
		Payout:      payout,
	}
	outputSize := payout.Sub(fc.Tax())
	fc.ValidProofOutputs = []types.SiacoinOutput{{Value: outputSize}}
	fc.MissedProofOutputs = []types.SiacoinOutput{{Value: outputSize}}

	// Create and fund a transaction with a file contract.
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(payout)
	if err != nil {
		return err
	}
	txnBuilder.AddFileContract(fc)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return err
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return err
	}
	block, _ = cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
	if err != nil {
		return err
	}
	if cst.cs.siafundPool.Cmp(types.NewCurrency64(15600e3).Add(oldSiafundPool)) != 0 {
		return errors.New("siafund pool did not update correctly")
	}

	// Create a transaction that spends siafunds.
	var claimDest types.UnlockHash
	_, err = rand.Read(claimDest[:])
	if err != nil {
		return err
	}
	var srcClaimStart types.Currency
	cst.cs.db.forEachSiafundOutputs(func(id types.SiafundOutputID, sfo types.SiafundOutput) {
		if sfo.UnlockHash == anyoneSpends {
			srcID = id
			srcValue = sfo.Value
			srcClaimStart = sfo.ClaimStart
		}
	})
	txn = types.Transaction{
		SiafundInputs: []types.SiafundInput{{
			ParentID:         srcID,
			UnlockConditions: types.UnlockConditions{},
			ClaimUnlockHash:  claimDest,
		}},
		SiafundOutputs: []types.SiafundOutput{
			{
				Value:      srcValue.Sub(types.NewCurrency64(1)),
				UnlockHash: types.UnlockConditions{}.UnlockHash(),
			},
			{
				Value:      types.NewCurrency64(1),
				UnlockHash: destAddr,
			},
		},
	}
	sfoid1 = txn.SiafundOutputID(1)
	cst.tpool.AcceptTransactionSet([]types.Transaction{txn})
	block, _ = cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
	if err != nil {
		return err
	}

	// Find the siafund output and check that it has the expected number of
	// siafunds.
	found := false
	expectedBalance := cst.cs.siafundPool.Sub(srcClaimStart).Div(types.NewCurrency64(10e3)).Mul(srcValue)
	for _, output := range cst.cs.delayedSiacoinOutputs[cst.cs.height()+types.MaturityDelay] {
		if output.UnlockHash == claimDest {
			found = true
			if output.Value.Cmp(expectedBalance) != 0 {
				return errors.New("siafund output has the wrong balance")
			}
		}
	}
	if !found {
		return errors.New("could not find siafund claim output")
	}

	return nil
}

// TestSpendSiafundsBlock creates a consensus set tester and uses it to call
// testSpendSiafundsBlock.
func TestSpendSiafundsBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestSpendSiafundsBlock")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// COMPATv0.4.0
	//
	// Mine enough blocks to get above the file contract hardfork threshold
	// (10).
	for i := 0; i < 10; i++ {
		block, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}
	err = cst.testSpendSiafundsBlock()
	if err != nil {
		t.Error(err)
	}
}

// testPaymentChannelBlocks submits blocks to set up, use, and close a payment
// channel.
func (cst *consensusSetTester) testPaymentChannelBlocks() error {
	// The current method of doing payment channels is gimped because public
	// keys do not have timelocks. We will be hardforking to include timelocks
	// in public keys in 0.4.0, but in the meantime we need an alternate
	// method.

	// Gimped payment channels: 2-of-2 multisig where one key is controlled by
	// the funding entity, and one key is controlled by the receiving entity. An
	// address is created containing both keys, and then the funding entity
	// creates, but does not sign, a transaction sending coins to the channel
	// address. A second transaction is created that sends all the coins in the
	// funding output back to the funding entity. The receiving entity signs the
	// transaction with a timelocked signature. The funding entity will get the
	// refund after T blocks as long as the output is not double spent. The
	// funding entity then signs the first transaction and opens the channel.
	//
	// Creating the channel:
	//	1. Create a 2-of-2 unlock conditions, one key held by each entity.
	//	2. Funding entity creates, but does not sign, a transaction sending
	//		money to the payment channel address. (txn A)
	//	3. Funding entity creates and signs a transaction spending the output
	//		created in txn A that sends all the money back as a refund. (txn B)
	//	4. Receiving entity signs txn B with a timelocked signature, so that the
	//		funding entity cannot get the refund for several days. The funding entity
	//		is given a fully signed and eventually-spendable txn B.
	//	5. The funding entity signs and broadcasts txn A.
	//
	// Using the channel:
	//	Each the receiving entity and the funding entity keeps a record of how
	//	much has been sent down the unclosed channel, and watches the
	//	blockchain for a channel closing transaction. To send more money down
	//	the channel, the funding entity creates and signs a transaction sending
	//	X+y coins to the receiving entity from the channel address. The
	//	transaction is sent to the receiving entity, who will keep it and
	//	potentially sign and broadcast it later. The funding entity will only
	//	send money down the channel if 'work' or some other sort of event has
	//	completed that indicates the receiving entity should get more money.
	//
	// Closing the channel:
	//	The receiving entity will sign the transaction that pays them the most
	//	money and then broadcast that transaction. This will spend the output
	//	and close the channel, invalidating txn B and preventing any future
	//	transactions from being made over the channel. The channel must be
	//	closed before the timelock expires on the second signature in txn B,
	//	otherwise the funding entity will be able to get a full refund.
	//
	//	The funding entity should be waiting until either the receiving entity
	//	closes the channel or the timelock expires. If the receiving entity
	//	closes the channel, all is good. If not, then the funding entity can
	//	close the channel and get a full refund.

	// Create a 2-of-2 unlock conditions, 1 key for each the sender and the
	// receiver in the payment channel.
	sk1, pk1, err := crypto.GenerateSignatureKeys() // Funding entity.
	if err != nil {
		return err
	}
	sk2, pk2, err := crypto.GenerateSignatureKeys() // Receiving entity.
	if err != nil {
		return err
	}
	uc := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{
			{
				Algorithm: types.SignatureEd25519,
				Key:       pk1[:],
			},
			{
				Algorithm: types.SignatureEd25519,
				Key:       pk2[:],
			},
		},
		SignaturesRequired: 2,
	}
	channelAddress := uc.UnlockHash()

	// Funding entity creates but does not sign a transaction that funds the
	// channel address. Because the wallet is not very flexible, the channel
	// txn needs to be fully custom. To get a custom txn, manually create an
	// address and then use the wallet to fund that address.
	channelSize := types.NewCurrency64(10e3)
	channelFundingSK, channelFundingPK, err := crypto.GenerateSignatureKeys()
	if err != nil {
		return err
	}
	channelFundingUC := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{{
			Algorithm: types.SignatureEd25519,
			Key:       channelFundingPK[:],
		}},
		SignaturesRequired: 1,
	}
	channelFundingAddr := channelFundingUC.UnlockHash()
	fundTxnBuilder := cst.wallet.StartTransaction()
	if err != nil {
		return err
	}
	err = fundTxnBuilder.FundSiacoins(channelSize)
	if err != nil {
		return err
	}
	scoFundIndex := fundTxnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: channelSize, UnlockHash: channelFundingAddr})
	fundTxnSet, err := fundTxnBuilder.Sign(true)
	if err != nil {
		return err
	}
	fundOutputID := fundTxnSet[len(fundTxnSet)-1].SiacoinOutputID(int(scoFundIndex))
	channelTxn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         fundOutputID,
			UnlockConditions: channelFundingUC,
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      channelSize,
			UnlockHash: channelAddress,
		}},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(fundOutputID),
			PublicKeyIndex: 0,
			CoveredFields:  types.CoveredFields{WholeTransaction: true},
		}},
	}

	// Funding entity creates and signs a transaction that spends the full
	// channel output.
	channelOutputID := channelTxn.SiacoinOutputID(0)
	refundUC, err := cst.wallet.NextAddress()
	refundAddr := refundUC.UnlockHash()
	if err != nil {
		return err
	}
	refundTxn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         channelOutputID,
			UnlockConditions: uc,
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      channelSize,
			UnlockHash: refundAddr,
		}},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(channelOutputID),
			PublicKeyIndex: 0,
			CoveredFields:  types.CoveredFields{WholeTransaction: true},
		}},
	}
	sigHash := refundTxn.SigHash(0)
	cryptoSig1, err := crypto.SignHash(sigHash, sk1)
	if err != nil {
		return err
	}
	refundTxn.TransactionSignatures[0].Signature = cryptoSig1[:]

	// Receiving entity signs the transaction that spends the full channel
	// output, but with a timelock.
	refundTxn.TransactionSignatures = append(refundTxn.TransactionSignatures, types.TransactionSignature{
		ParentID:       crypto.Hash(channelOutputID),
		PublicKeyIndex: 1,
		Timelock:       cst.cs.height() + 2,
		CoveredFields:  types.CoveredFields{WholeTransaction: true},
	})
	sigHash = refundTxn.SigHash(1)
	cryptoSig2, err := crypto.SignHash(sigHash, sk2)
	if err != nil {
		return err
	}
	refundTxn.TransactionSignatures[1].Signature = cryptoSig2[:]

	// Funding entity will now sign and broadcast the funding transaction.
	sigHash = channelTxn.SigHash(0)
	cryptoSig0, err := crypto.SignHash(sigHash, channelFundingSK)
	if err != nil {
		return err
	}
	channelTxn.TransactionSignatures[0].Signature = cryptoSig0[:]
	err = cst.tpool.AcceptTransactionSet(append(fundTxnSet, channelTxn))
	if err != nil {
		return err
	}
	// Put the txn in a block.
	block, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
	if err != nil {
		return err
	}

	// Try to submit the refund transaction before the timelock has expired.
	err = cst.tpool.AcceptTransactionSet([]types.Transaction{refundTxn})
	if err != types.ErrPrematureSignature {
		return err
	}

	// Create a transaction that has partially used the channel, and submit it
	// to the blockchain to close the channel.
	closeTxn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         channelOutputID,
			UnlockConditions: uc,
		}},
		SiacoinOutputs: []types.SiacoinOutput{
			{
				Value:      channelSize.Sub(types.NewCurrency64(5)),
				UnlockHash: refundAddr,
			},
			{
				Value: types.NewCurrency64(5),
			},
		},
		TransactionSignatures: []types.TransactionSignature{
			{
				ParentID:       crypto.Hash(channelOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			},
			{
				ParentID:       crypto.Hash(channelOutputID),
				PublicKeyIndex: 1,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			},
		},
	}
	sigHash = closeTxn.SigHash(0)
	cryptoSig3, err := crypto.SignHash(sigHash, sk1)
	if err != nil {
		return err
	}
	closeTxn.TransactionSignatures[0].Signature = cryptoSig3[:]
	sigHash = closeTxn.SigHash(1)
	cryptoSig4, err := crypto.SignHash(sigHash, sk2)
	if err != nil {
		return err
	}
	closeTxn.TransactionSignatures[1].Signature = cryptoSig4[:]
	err = cst.tpool.AcceptTransactionSet([]types.Transaction{closeTxn})
	if err != nil {
		return err
	}

	// Mine the block with the transaction.
	block, _ = cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
	if err != nil {
		return err
	}
	closeRefundID := closeTxn.SiacoinOutputID(0)
	closePaymentID := closeTxn.SiacoinOutputID(1)
	exists := cst.cs.db.inSiacoinOutputs(closeRefundID)
	if !exists {
		return errors.New("close txn refund output doesn't exist")
	}
	exists = cst.cs.db.inSiacoinOutputs(closePaymentID)
	if !exists {
		return errors.New("close txn payment output doesn't exist")
	}

	// Create a payment channel where the receiving entity never responds to
	// the initial transaction.
	{
		// Funding entity creates but does not sign a transaction that funds the
		// channel address. Because the wallet is not very flexible, the channel
		// txn needs to be fully custom. To get a custom txn, manually create an
		// address and then use the wallet to fund that address.
		channelSize := types.NewCurrency64(10e3)
		channelFundingSK, channelFundingPK, err := crypto.GenerateSignatureKeys()
		if err != nil {
			return err
		}
		channelFundingUC := types.UnlockConditions{
			PublicKeys: []types.SiaPublicKey{{
				Algorithm: types.SignatureEd25519,
				Key:       channelFundingPK[:],
			}},
			SignaturesRequired: 1,
		}
		channelFundingAddr := channelFundingUC.UnlockHash()
		fundTxnBuilder := cst.wallet.StartTransaction()
		err = fundTxnBuilder.FundSiacoins(channelSize)
		if err != nil {
			return err
		}
		scoFundIndex := fundTxnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: channelSize, UnlockHash: channelFundingAddr})
		fundTxnSet, err := fundTxnBuilder.Sign(true)
		if err != nil {
			return err
		}
		fundOutputID := fundTxnSet[len(fundTxnSet)-1].SiacoinOutputID(int(scoFundIndex))
		channelTxn := types.Transaction{
			SiacoinInputs: []types.SiacoinInput{{
				ParentID:         fundOutputID,
				UnlockConditions: channelFundingUC,
			}},
			SiacoinOutputs: []types.SiacoinOutput{{
				Value:      channelSize,
				UnlockHash: channelAddress,
			}},
			TransactionSignatures: []types.TransactionSignature{{
				ParentID:       crypto.Hash(fundOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			}},
		}

		// Funding entity creates and signs a transaction that spends the full
		// channel output.
		channelOutputID := channelTxn.SiacoinOutputID(0)
		refundUC, err := cst.wallet.NextAddress()
		refundAddr := refundUC.UnlockHash()
		if err != nil {
			return err
		}
		refundTxn := types.Transaction{
			SiacoinInputs: []types.SiacoinInput{{
				ParentID:         channelOutputID,
				UnlockConditions: uc,
			}},
			SiacoinOutputs: []types.SiacoinOutput{{
				Value:      channelSize,
				UnlockHash: refundAddr,
			}},
			TransactionSignatures: []types.TransactionSignature{{
				ParentID:       crypto.Hash(channelOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			}},
		}
		sigHash := refundTxn.SigHash(0)
		cryptoSig1, err := crypto.SignHash(sigHash, sk1)
		if err != nil {
			return err
		}
		refundTxn.TransactionSignatures[0].Signature = cryptoSig1[:]

		// Recieving entity never communitcates, funding entity must reclaim
		// the 'channelSize' coins that were intended to go to the channel.
		reclaimUC, err := cst.wallet.NextAddress()
		reclaimAddr := reclaimUC.UnlockHash()
		if err != nil {
			return err
		}
		reclaimTxn := types.Transaction{
			SiacoinInputs: []types.SiacoinInput{{
				ParentID:         fundOutputID,
				UnlockConditions: channelFundingUC,
			}},
			SiacoinOutputs: []types.SiacoinOutput{{
				Value:      channelSize,
				UnlockHash: reclaimAddr,
			}},
			TransactionSignatures: []types.TransactionSignature{{
				ParentID:       crypto.Hash(fundOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			}},
		}
		sigHash = reclaimTxn.SigHash(0)
		cryptoSig, err := crypto.SignHash(sigHash, channelFundingSK)
		if err != nil {
			return err
		}
		reclaimTxn.TransactionSignatures[0].Signature = cryptoSig[:]
		err = cst.tpool.AcceptTransactionSet(append(fundTxnSet, reclaimTxn))
		if err != nil {
			return err
		}
		block, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			return err
		}
		reclaimOutputID := reclaimTxn.SiacoinOutputID(0)
		exists := cst.cs.db.inSiacoinOutputs(reclaimOutputID)
		if !exists {
			return errors.New("failed to reclaim an output that belongs to the funding entity")
		}
	}

	// Create a channel and the open the channel, but close the channel using
	// the timelocked signature.
	{
		// Funding entity creates but does not sign a transaction that funds the
		// channel address. Because the wallet is not very flexible, the channel
		// txn needs to be fully custom. To get a custom txn, manually create an
		// address and then use the wallet to fund that address.
		channelSize := types.NewCurrency64(10e3)
		channelFundingSK, channelFundingPK, err := crypto.GenerateSignatureKeys()
		if err != nil {
			return err
		}
		channelFundingUC := types.UnlockConditions{
			PublicKeys: []types.SiaPublicKey{{
				Algorithm: types.SignatureEd25519,
				Key:       channelFundingPK[:],
			}},
			SignaturesRequired: 1,
		}
		channelFundingAddr := channelFundingUC.UnlockHash()
		fundTxnBuilder := cst.wallet.StartTransaction()
		err = fundTxnBuilder.FundSiacoins(channelSize)
		if err != nil {
			return err
		}
		scoFundIndex := fundTxnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: channelSize, UnlockHash: channelFundingAddr})
		fundTxnSet, err := fundTxnBuilder.Sign(true)
		if err != nil {
			return err
		}
		fundOutputID := fundTxnSet[len(fundTxnSet)-1].SiacoinOutputID(int(scoFundIndex))
		channelTxn := types.Transaction{
			SiacoinInputs: []types.SiacoinInput{{
				ParentID:         fundOutputID,
				UnlockConditions: channelFundingUC,
			}},
			SiacoinOutputs: []types.SiacoinOutput{{
				Value:      channelSize,
				UnlockHash: channelAddress,
			}},
			TransactionSignatures: []types.TransactionSignature{{
				ParentID:       crypto.Hash(fundOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			}},
		}

		// Funding entity creates and signs a transaction that spends the full
		// channel output.
		channelOutputID := channelTxn.SiacoinOutputID(0)
		refundUC, err := cst.wallet.NextAddress()
		refundAddr := refundUC.UnlockHash()
		if err != nil {
			return err
		}
		refundTxn := types.Transaction{
			SiacoinInputs: []types.SiacoinInput{{
				ParentID:         channelOutputID,
				UnlockConditions: uc,
			}},
			SiacoinOutputs: []types.SiacoinOutput{{
				Value:      channelSize,
				UnlockHash: refundAddr,
			}},
			TransactionSignatures: []types.TransactionSignature{{
				ParentID:       crypto.Hash(channelOutputID),
				PublicKeyIndex: 0,
				CoveredFields:  types.CoveredFields{WholeTransaction: true},
			}},
		}
		sigHash := refundTxn.SigHash(0)
		cryptoSig1, err := crypto.SignHash(sigHash, sk1)
		if err != nil {
			return err
		}
		refundTxn.TransactionSignatures[0].Signature = cryptoSig1[:]

		// Receiving entity signs the transaction that spends the full channel
		// output, but with a timelock.
		refundTxn.TransactionSignatures = append(refundTxn.TransactionSignatures, types.TransactionSignature{
			ParentID:       crypto.Hash(channelOutputID),
			PublicKeyIndex: 1,
			Timelock:       cst.cs.height() + 2,
			CoveredFields:  types.CoveredFields{WholeTransaction: true},
		})
		sigHash = refundTxn.SigHash(1)
		cryptoSig2, err := crypto.SignHash(sigHash, sk2)
		if err != nil {
			return err
		}
		refundTxn.TransactionSignatures[1].Signature = cryptoSig2[:]

		// Funding entity will now sign and broadcast the funding transaction.
		sigHash = channelTxn.SigHash(0)
		cryptoSig0, err := crypto.SignHash(sigHash, channelFundingSK)
		if err != nil {
			return err
		}
		channelTxn.TransactionSignatures[0].Signature = cryptoSig0[:]
		err = cst.tpool.AcceptTransactionSet(append(fundTxnSet, channelTxn))
		if err != nil {
			return err
		}
		// Put the txn in a block.
		block, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			return err
		}

		// Receiving entity never signs another transaction, so the funding
		// entity waits until the timelock is complete, and then submits the
		// refundTxn.
		for i := 0; i < 3; i++ {
			block, _ := cst.miner.FindBlock()
			err = cst.cs.AcceptBlock(block)
			if err != nil {
				return err
			}
		}
		err = cst.tpool.AcceptTransactionSet([]types.Transaction{refundTxn})
		if err != nil {
			return err
		}
		block, _ = cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			return err
		}
		refundOutputID := refundTxn.SiacoinOutputID(0)
		exists := cst.cs.db.inSiacoinOutputs(refundOutputID)
		if !exists {
			return errors.New("timelocked refund transaction did not get spent correctly")
		}
	}

	return nil
}

// TestPaymentChannelBlocks creates a consensus set tester and uses it to call
// testPaymentChannelBlocks.
func TestPaymentChannelBlocks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestPaymentChannelBlocks")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	err = cst.testPaymentChannelBlocks()
	if err != nil {
		t.Fatal(err)
	}
}

// complexBlockSet puts a set of blocks with many types of transactions into
// the consensus set.
func (cst *consensusSetTester) complexBlockSet() error {
	err := cst.testSimpleBlock()
	if err != nil {
		return err
	}
	err = cst.testSpendSiacoinsBlock()
	if err != nil {
		return err
	}

	// COMPATv0.4.0
	//
	// Mine enough blocks to get above the file contract hardfork threshold
	// (10).
	for i := 0; i < 10; i++ {
		block, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			return err
		}
	}

	err = cst.testFileContractsBlocks()
	if err != nil {
		return err
	}
	err = cst.testSpendSiafundsBlock()
	if err != nil {
		return err
	}
	return nil
}

// TestComplexForking adds every type of test block into two parallel chains of
// consensus, and then forks to a new chain, forcing the whole structure to be
// reverted.
func TestComplexForking(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst1, err := createConsensusSetTester("TestComplexForking - 1")
	if err != nil {
		t.Fatal(err)
	}
	defer cst1.closeCst()
	cst2, err := createConsensusSetTester("TestComplexForking - 2")
	if err != nil {
		t.Fatal(err)
	}
	defer cst2.closeCst()
	cst3, err := createConsensusSetTester("TestComplexForking - 3")
	if err != nil {
		t.Fatal(err)
	}
	defer cst3.closeCst()

	// Give each type of major block to cst1.
	err = cst1.complexBlockSet()
	if err != nil {
		t.Fatal(err)
	}

	// Give all the blocks in cst1 to cst3 - as a holding place.
	var cst1Blocks []types.Block
	pb := cst1.cs.currentProcessedBlock()
	for pb.Block.ID() != cst1.cs.blockRoot.Block.ID() {
		cst1Blocks = append([]types.Block{pb.Block}, cst1Blocks...) // prepend
		pb = cst1.cs.db.getBlockMap(pb.Parent)
	}

	for _, block := range cst1Blocks {
		// Some blocks will return errors.
		err = cst3.cs.AcceptBlock(block)
		if err == nil {
		}
	}
	if cst3.cs.currentBlockID() != cst1.cs.currentBlockID() {
		t.Error("cst1 and cst3 do not share the same path")
	}
	if cst3.cs.consensusSetHash() != cst1.cs.consensusSetHash() {
		t.Error("cst1 and cst3 do not share a consensus set hash")
	}

	// Mine 3 blocks on cst2, then all the block types, to give it a heavier
	// weight, then give all of its blocks to cst1. This will cause a complex
	// fork to happen.
	for i := 0; i < 3; i++ {
		block, _ := cst2.miner.FindBlock()
		err = cst2.cs.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}
	err = cst2.complexBlockSet()
	if err != nil {
		t.Fatal(err)
	}
	var cst2Blocks []types.Block
	pb = cst2.cs.currentProcessedBlock()
	for pb.Block.ID() != cst2.cs.blockRoot.Block.ID() {
		cst2Blocks = append([]types.Block{pb.Block}, cst2Blocks...) // prepend
		pb = cst2.cs.db.getBlockMap(pb.Parent)
	}
	for _, block := range cst2Blocks {
		// Some blocks will return errors.
		_ = cst1.cs.AcceptBlock(block)
	}
	if cst1.cs.currentBlockID() != cst2.cs.currentBlockID() {
		t.Error("cst1 and cst2 do not share the same path")
	}
	if cst1.cs.consensusSetHash() != cst2.cs.consensusSetHash() {
		t.Error("cst1 and cst2 do not share the same consensus set hash")
	}

	// Mine 6 blocks on cst3 and then give those blocks to cst1, which will
	// cause cst1 to switch back to its old chain. cst1 will then have created,
	// reverted, and reapplied all the significant types of blocks.
	for i := 0; i < 6; i++ {
		block, _ := cst3.miner.FindBlock()
		err = cst3.cs.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}
	var cst3Blocks []types.Block
	pb = cst3.cs.currentProcessedBlock()
	for pb.Block.ID() != cst3.cs.blockRoot.Block.ID() {
		cst3Blocks = append([]types.Block{pb.Block}, cst3Blocks...) // prepend
		pb = cst3.cs.db.getBlockMap(pb.Parent)
	}
	for _, block := range cst3Blocks {
		// Some blocks will return errors.
		err = cst1.cs.AcceptBlock(block)
		if err == nil {
		}
	}
	if cst1.cs.currentBlockID() != cst3.cs.currentBlockID() {
		t.Error("cst1 and cst3 do not share the same path")
	}
	if cst1.cs.consensusSetHash() != cst3.cs.consensusSetHash() {
		t.Error("cst1 and cst3 do not share the same consensus set hash")
	}
}

// TestBuriedBadFork creates a block with an invalid transaction that's not on
// the longest fork. The consensus set will not validate that block. Then valid
// blocks are added on top of it to make it the longest fork. When it becomes
// the longest fork, all the blocks should be fully validated and thrown out
// because a parent is invalid.
func TestBuriedBadFork(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	cst, err := createConsensusSetTester("TestBuriedBadFork")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	pb := cst.cs.currentProcessedBlock()

	// Create a bad block that builds on a parent, so that it is part of not
	// the longest fork.
	badBlock := types.Block{
		ParentID:     pb.Parent,
		Timestamp:    types.CurrentTimestamp(),
		MinerPayouts: []types.SiacoinOutput{{Value: types.CalculateCoinbase(pb.Height)}},
		Transactions: []types.Transaction{{
			SiacoinInputs: []types.SiacoinInput{{}}, // Will trigger an error on full verification but not partial verification.
		}},
	}
	parent := cst.cs.db.getBlockMap(pb.Parent)
	badBlock, _ = cst.miner.SolveBlock(badBlock, parent.ChildTarget)
	err = cst.cs.AcceptBlock(badBlock)
	if err != modules.ErrNonExtendingBlock {
		t.Fatal(err)
	}

	// Build another bock on top of the bad block that is fully valid, this
	// will cause a fork and full validation of the bad block, both the bad
	// block and this block should be thrown away.
	block := types.Block{
		ParentID:     badBlock.ID(),
		Timestamp:    types.CurrentTimestamp(),
		MinerPayouts: []types.SiacoinOutput{{Value: types.CalculateCoinbase(pb.Height + 1)}},
	}
	block, _ = cst.miner.SolveBlock(block, parent.ChildTarget) // okay because the target will not change
	err = cst.cs.AcceptBlock(block)
	if err == nil {
		t.Fatal(err)
	}
	exists := cst.cs.db.inBlockMap(badBlock.ID())
	if exists {
		t.Error("bad block not cleared from memory")
	}
	exists = cst.cs.db.inBlockMap(block.ID())
	if exists {
		t.Error("block not cleared from memory")
	}
}

// TestBuriedBadTransaction tries submitting a block with a bad transaction
// that is buried under good transactions.
func TestBuriedBadTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestBuriedBadTransaction")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()
	pb := cst.cs.currentProcessedBlock()

	// Create a good transaction using the wallet.
	txnValue := types.NewCurrency64(1200)
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(txnValue)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddSiacoinOutput(types.SiacoinOutput{Value: txnValue})
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal(err)
	}

	// Create a bad transaction
	badTxn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{}},
	}
	txns := append(cst.tpool.TransactionList(), badTxn)

	// Create a block with a buried bad transaction.
	block := types.Block{
		ParentID:     pb.Block.ID(),
		Timestamp:    types.CurrentTimestamp(),
		MinerPayouts: []types.SiacoinOutput{{Value: types.CalculateCoinbase(pb.Height + 1)}},
		Transactions: txns,
	}
	block, _ = cst.miner.SolveBlock(block, pb.ChildTarget)
	err = cst.cs.AcceptBlock(block)
	if err == nil {
		t.Error("buried transaction didn't cause an error")
	}
	exists := cst.cs.db.inBlockMap(block.ID())
	if exists {
		t.Error("bad block made it into the block map")
	}
}

// COMPATv0.4.0
//
// This test checks that the hardfork scheduled for block 12,000 rolls through
// smoothly.
func TestTaxHardfork(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestTaxHardfork")
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

	// Create a file contract with a payout that is put into the blockchain
	// before the hardfork block but expires after the hardfork block.
	payout := types.NewCurrency64(400e6)
	fc := types.FileContract{
		WindowStart:        cst.cs.height() + 10,
		WindowEnd:          cst.cs.height() + 12,
		Payout:             payout,
		ValidProofOutputs:  []types.SiacoinOutput{{}},
		MissedProofOutputs: []types.SiacoinOutput{{}},
	}
	outputSize := payout.Sub(fc.Tax())
	fc.ValidProofOutputs[0].Value = outputSize
	fc.MissedProofOutputs[0].Value = outputSize

	// Create and fund a transaction with a file contract.
	txnBuilder := cst.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(payout)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddFileContract(fc)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(block)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the siafund pool was increased.
	if cst.cs.siafundPool.Cmp(types.NewCurrency64(15590e3)) != 0 {
		t.Fatal("siafund pool was not increased correctly")
	}

	// Mine blocks until the file contract expires and see if any problems
	// occur.
	for i := 0; i < 12; i++ {
		block, _ := cst.miner.FindBlock()
		err = cst.cs.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Run a siacoins check to make sure that order has been restored - note
	// that the siacoin check will fail in the middle, and thus is commented
	// out until after the hardfork.
	err = cst.cs.checkSiacoins()
	if err != nil {
		t.Fatal(err)
	}
}
