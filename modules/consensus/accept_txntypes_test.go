package consensus

import (
	"testing"

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
