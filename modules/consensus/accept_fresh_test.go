package consensus

import (
	"errors"
	"testing"
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
