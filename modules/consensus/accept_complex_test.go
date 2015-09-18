package consensus

import (
	"fmt"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

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
		t.Error(err)
	}

	// Give all the blocks in cst1 to cst3 - as a holding place.
	var cst1Blocks []types.Block
	pb := cst1.cs.currentProcessedBlock()
	for pb.Block.ID() != cst1.cs.blockRoot.Block.ID() {
		cst1Blocks = append([]types.Block{pb.Block}, cst1Blocks...) // prepend
		pb = cst1.cs.db.getBlockMap(pb.Block.ParentID)
	}

	for _, block := range cst1Blocks {
		// Some blocks will return errors.
		_ = cst3.cs.AcceptBlock(block)
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
		t.Error(err)
	}
	var cst2Blocks []types.Block
	pb = cst2.cs.currentProcessedBlock()
	for pb.Block.ID() != cst2.cs.blockRoot.Block.ID() {
		cst2Blocks = append([]types.Block{pb.Block}, cst2Blocks...) // prepend
		pb = cst2.cs.db.getBlockMap(pb.Block.ParentID)
	}
	fmt.Println(cst1.cs.dbBlockHeight())
	for i, block := range cst2Blocks {
		// Some blocks will return errors.
		fmt.Println(i, cst1.cs.dbBlockHeight())
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
		pb = cst3.cs.db.getBlockMap(pb.Block.ParentID)
	}
	for _, block := range cst3Blocks {
		// Some blocks will return errors.
		_ = cst1.cs.AcceptBlock(block)
	}
	if cst1.cs.currentBlockID() != cst3.cs.currentBlockID() {
		t.Error("cst1 and cst3 do not share the same path")
	}
	if cst1.cs.consensusSetHash() != cst3.cs.consensusSetHash() {
		t.Error("cst1 and cst3 do not share the same consensus set hash")
	}
}
