package consensus

import (
	"errors"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// testSimpleBlock mines a simple block (no transactions except those
// automatically added by the miner) and adds it to the consnesus set.
func (cst *consensusSetTester) testSimpleBlock() error {
	// Get the starting hash of the consenesus set.
	initialCSSum := cst.cs.consensusSetHash()

	// Mine and submit a block
	block, _, err := cst.miner.FindBlock()
	if err != nil {
		return err
	}
	cst.csUpdateWait()

	// Get the ending hash of the consensus set.
	resultingCSSum := cst.cs.consensusSetHash()
	if initialCSSum == resultingCSSum {
		return errors.New("state hash is unchanged after mining a block")
	}

	// Check that the current path has updated as expected.
	newNode := cst.cs.currentBlockNode()
	if cst.cs.CurrentBlock().ID() != block.ID() {
		return errors.New("the state's current block is not reporting as the recently mined block.")
	}
	// Check that the current path has updated correctly.
	if block.ID() != cst.cs.currentPath[newNode.height] {
		return errors.New("the state's current path didn't update correctly after accepting a new block")
	}

	// Revert the block that was just added to the consensus set and check for
	// parity with the original state of consensus.
	_, _, err = cst.cs.forkBlockchain(newNode.parent)
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

// TestSimpleBlock is a passthrough function.
func TestSimpleBlock(t *testing.T) {
	cst, err := createConsensusSetTester("TestSimpleBlock")
	if err != nil {
		t.Fatal(err)
	}
	err = cst.testSimpleBlock()
	if err != nil {
		t.Error(err)
	}
}

// testDoSBlockHandling checks that saved bad blocks are correctly ignored.
func (cst *consensusSetTester) testDoSBlockHandling() error {
	// Mine a DoS block and submit it to the state, expect a normal error.
	dosBlock, err := cst.MineDoSBlock()
	if err != nil {
		return err
	}
	err = cst.cs.acceptBlock(dosBlock)
	// The error is mostly irrelevant, it just needs to have the block flagged
	// as a DoS block in future attempts.
	if err != ErrSiacoinInputOutputMismatch {
		return errors.New("expecting invalid signature err: " + err.Error())
	}

	// Submit the same DoS block to the state again, expect ErrDoSBlock.
	err = cst.cs.acceptBlock(dosBlock)
	if err != ErrDoSBlock {
		return errors.New("expecting bad block err: " + err.Error())
	}
	return nil
}

// TestDoSBlockHandling creates a new consensus set tester and uses it to call
// testDoSBlockHandling.
func TestDoSBlockHandling(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	cst, err := createConsensusSetTester("TestDoSBlockHandling")
	if err != nil {
		t.Fatal(err)
	}
	err = cst.testDoSBlockHandling()
	if err != nil {
		t.Error(err)
	}
}

// testBlockKnownHandling submits known blocks to the consensus set.
func (cst *consensusSetTester) testBlockKnownHandling() error {
	// Get a block destined to be stale.
	block, _, target := cst.miner.BlockForWork()
	staleBlock, _ := cst.miner.SolveBlock(block, target)

	// Add two new blocks to the consensus set to block the stale block.
	block1, _, err := cst.miner.FindBlock()
	if err != nil {
		return err
	}
	cst.csUpdateWait()
	block2, _, err := cst.miner.FindBlock()
	if err != nil {
		return err
	}
	cst.csUpdateWait()

	// Submit the stale block.
	err = cst.cs.acceptBlock(staleBlock)
	if err != nil {
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
	genesisBlock := cst.cs.blockMap[cst.cs.currentPath[0]].block
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

	// Mine a block that doesn't meet the target.
	block, _, target := cst.miner.BlockForWork()
	for block.CheckTarget(target) {
		block.Nonce++
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

	// Create a transaction that puts the block over the size limit.
	bigData := string(make([]byte, types.BlockSizeLimit))
	txn := types.Transaction{
		ArbitraryData: []string{bigData},
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

	// Create a block with a too early timestamp - block should be rejected
	// outright.
	block, _, target := cst.miner.BlockForWork()
	earliestTimestamp := cst.cs.blockMap[block.ParentID].earliestChildTimestamp()
	block.Timestamp = earliestTimestamp - 1
	earlyBlock, _ := cst.miner.SolveBlock(block, target)
	err = cst.cs.acceptBlock(earlyBlock)
	if err != ErrEarlyTimestamp {
		t.Error("expecting ErrEarlyTimestamp:", err.Error())
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

// Create a block with a timestamp in the future - the block should be held
// until that future has arrived, and then it should be added to the
// consensus set. If the timestamp is more than ExtremeFutureThreshold then
// the block should be discarded outright.
// types.CurrentTimestamp(), types.FutureThreshold, types.ExtremeFutureThreshold
