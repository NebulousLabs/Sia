package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// reorgSets contains multiple consensus sets that share a genesis block, which
// can be manipulated to cause full integration blockchain reorgs.
//
// cstBackup is a holding place for cstMain - the blocks originally in cstMain get moved
// to cstBackup so that cstMain can be reorganized without that history being lost.
// Extending cstBackup will allow cstMain to be reorg'd back to its original blocks.
type reorgSets struct {
	cstMain   *consensusSetTester
	cstAlt    *consensusSetTester
	cstBackup *consensusSetTester
}

// createReorgSets creates a reorg set that is ready to be manipulated.
func createReorgSets(name string) *reorgSets {
	cstMain, err := createConsensusSetTester(name + " - 1")
	if err != nil {
		panic(err)
	}
	defer cstMain.closeCst()
	cstAlt, err := createConsensusSetTester(name + " - 2")
	if err != nil {
		panic(err)
	}
	defer cstAlt.closeCst()
	cstBackup, err := createConsensusSetTester(name + " - 3")
	if err != nil {
		panic(err)
	}
	defer cstBackup.closeCst()

	return &reorgSets{
		cstMain:   cstMain,
		cstAlt:    cstAlt,
		cstBackup: cstBackup,
	}
}

// save takes all of the blocks in cstMain and moves them to cstBackup.
func (rs *reorgSets) save() {
	mainHeight := rs.cstMain.cs.dbBlockHeight()
	for i := types.BlockHeight(1); i <= mainHeight; i++ {
		id, err := rs.cstMain.cs.dbGetPath(i)
		if err != nil {
			panic(err)
		}
		pb, err := rs.cstMain.cs.dbGetBlockMap(id)
		if err != nil {
			panic(err)
		}

		// err is not checked - block may already be in cstBackup.
		_ = rs.cstBackup.cs.AcceptBlock(pb.Block)
	}

	// Check that cstMain and cstBackup are even.
	if rs.cstMain.cs.dbCurrentProcessedBlock().Block.ID() != rs.cstBackup.cs.dbCurrentProcessedBlock().Block.ID() {
		panic("could not save cstMain into cstBackup")
	}
	if rs.cstMain.cs.dbConsensusChecksum() != rs.cstBackup.cs.dbConsensusChecksum() {
		panic("reorg checksums do not match after saving")
	}
}

// extend adds blocks to cstAlt until cstAlt has more weight than cstMain. Then
// cstMain is caught up, causing cstMain to perform a reorg that extends all
// the way to the genesis block.
func (rs *reorgSets) extend() {
	for rs.cstMain.cs.dbBlockHeight() >= rs.cstAlt.cs.dbBlockHeight() {
		_, err := rs.cstAlt.miner.AddBlock()
		if err != nil {
			panic(err)
		}
	}
	for i := types.BlockHeight(1); i <= rs.cstAlt.cs.dbBlockHeight(); i++ {
		id, err := rs.cstAlt.cs.dbGetPath(i)
		if err != nil {
			panic(err)
		}
		pb, err := rs.cstAlt.cs.dbGetBlockMap(id)
		if err != nil {
			panic(err)
		}
		_ = rs.cstMain.cs.AcceptBlock(pb.Block)
	}

	// Check that cstMain and cstAlt are even.
	if rs.cstMain.cs.dbCurrentProcessedBlock().Block.ID() != rs.cstAlt.cs.dbCurrentProcessedBlock().Block.ID() {
		panic("could not save cstMain into cstAlt")
	}
	if rs.cstMain.cs.dbConsensusChecksum() != rs.cstAlt.cs.dbConsensusChecksum() {
		panic("reorg checksums do not match after extending")
	}
}

// restore extends cstBackup until it is ahead of cstMain, and then adds all of
// the blocks from cstBackup to cstMain, causing cstMain to reorg to the state
// of cstBackup.
func (rs *reorgSets) restore() {
	for rs.cstMain.cs.dbBlockHeight() >= rs.cstBackup.cs.dbBlockHeight() {
		_, err := rs.cstBackup.miner.AddBlock()
		if err != nil {
			panic(err)
		}
	}
	for i := types.BlockHeight(1); i <= rs.cstBackup.cs.dbBlockHeight(); i++ {
		id, err := rs.cstBackup.cs.dbGetPath(i)
		if err != nil {
			panic(err)
		}
		pb, err := rs.cstBackup.cs.dbGetBlockMap(id)
		if err != nil {
			panic(err)
		}
		_ = rs.cstMain.cs.AcceptBlock(pb.Block)
	}

	// Check that cstMain and cstBackup are even.
	if rs.cstMain.cs.dbCurrentProcessedBlock().Block.ID() != rs.cstBackup.cs.dbCurrentProcessedBlock().Block.ID() {
		panic("could not save cstMain into cstBackup")
	}
	if rs.cstMain.cs.dbConsensusChecksum() != rs.cstBackup.cs.dbConsensusChecksum() {
		panic("reorg checksums do not match after restoring")
	}
}

// fullReorg saves all of the blocks from cstMain into cstBackup, then extends
// cstAlt until cstMain joins cstAlt in structure. Then cstBackup is extended
// and cstMain is reorg'd back to have all of the original blocks.
func (rs *reorgSets) fullReorg() {
	rs.save()
	rs.extend()
	rs.restore()
}

// TestIntegrationSimpleReorg tries to reorganize a simple block out of, and
// then back into, the consensus set.
func TestIntegrationSimpleReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rs := createReorgSets("TestIntegrationSimpleReorg")

	// Give a simple block to cstMain.
	rs.cstMain.testSimpleBlock()

	// Try to trigger consensus inconsistencies by doing a full reorg on the
	// simple block.
	rs.fullReorg()
}

// TestIntegrationSiacoinReorg tries to reorganize a siacoin output block out
// of, and then back into, the consensus set.
func TestIntegrationSiacoinReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rs := createReorgSets("TestIntegrationSiacoinReorg")

	// Give a siacoin block to cstMain.
	rs.cstMain.testSpendSiacoinsBlock()

	// Try to trigger consensus inconsistencies by doing a full reorg on the
	// simple block.
	rs.fullReorg()
}

// TestIntegrationValidStorageProofReorg tries to reorganize a valid storage
// proof block out of, and then back into, the consensus set.
func TestIntegrationValidStorageProofReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rs := createReorgSets("TestIntegrationValidStorageProofReorg")

	// Give a series of blocks containing a file contract and a valid storage
	// proof to cstMain.
	rs.cstMain.testValidStorageProofBlocks()

	// Try to trigger consensus inconsistencies by doing a full reorg on the
	// simple block.
	rs.fullReorg()
}

// TestIntegrationMissedStorageProofReorg tries to reorganize a valid storage
// proof block out of, and then back into, the consensus set.
func TestIntegrationMissedStorageProofReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rs := createReorgSets("TestIntegrationMissedStorageProofReorg")

	// Give a series of blocks containing a file contract and a valid storage
	// proof to cstMain.
	rs.cstMain.testMissedStorageProofBlocks()

	// Try to trigger consensus inconsistencies by doing a full reorg on the
	// simple block.
	rs.fullReorg()
}

// TestIntegrationComplexReorg stacks up blocks of all types into a single
// blockchain that undergoes a massive reorg as a stress test to the codebase.
func TestIntegrationComplexReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rs := createReorgSets("TestIntegrationComplexReorg")

	// Give a wide variety of block types to cstMain.
	for i := 0; i < 3; i++ {
		rs.cstMain.testBlockSuite()
	}
	// Give fewer blocks to cstAlt, while still using the same variety.
	for i := 0; i < 2; i++ {
		rs.cstAlt.testBlockSuite()
	}

	// Try to trigger consensus inconsistencies by doing a full reorg on the
	// simple block.
	rs.fullReorg()
}

/// All functions below this point are deprecated. ///

// complexBlockSet puts a set of blocks with many types of transactions into
// the consensus set.
//
// complexBlockSet can be deleted once equivalent functionality has been added
// to 'testBlockSuite', which is currently missing file contract revisions and
// all forms of siafund testing.
func (cst *consensusSetTester) complexBlockSet() error {
	cst.testSimpleBlock()
	cst.testSpendSiacoinsBlock()

	// COMPATv0.4.0
	//
	// Mine enough blocks to get above the file contract hardfork threshold
	// (10).
	for i := 0; i < 10; i++ {
		block, _ := cst.miner.FindBlock()
		err := cst.cs.AcceptBlock(block)
		if err != nil {
			return err
		}
	}

	// err := cst.testFileContractsBlocks()
	// err = cst.testSpendSiafundsBlock()
	return nil
}

// TestComplexForking adds every type of test block into two parallel chains of
// consensus, and then forks to a new chain, forcing the whole structure to be
// reverted.
//
// testComplexForking can be removed once testBlockSuite is complete.
func TestComplexForking(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cstMain, err := createConsensusSetTester("TestComplexForking - 1")
	if err != nil {
		t.Fatal(err)
	}
	defer cstMain.closeCst()
	cstAlt, err := createConsensusSetTester("TestComplexForking - 2")
	if err != nil {
		t.Fatal(err)
	}
	defer cstAlt.closeCst()
	cstBackup, err := createConsensusSetTester("TestComplexForking - 3")
	if err != nil {
		t.Fatal(err)
	}
	defer cstBackup.closeCst()

	// Give each type of major block to cstMain.
	err = cstMain.complexBlockSet()
	if err != nil {
		t.Error(err)
	}

	// Give all the blocks in cstMain to cstBackup - as a holding place.
	var cstMainBlocks []types.Block
	pb := cstMain.cs.dbCurrentProcessedBlock()
	for pb.Block.ID() != cstMain.cs.blockRoot.Block.ID() {
		cstMainBlocks = append([]types.Block{pb.Block}, cstMainBlocks...) // prepend
		pb, err = cstMain.cs.dbGetBlockMap(pb.Block.ParentID)
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, block := range cstMainBlocks {
		// Some blocks will return errors.
		_ = cstBackup.cs.AcceptBlock(block)
	}
	if cstBackup.cs.dbCurrentBlockID() != cstMain.cs.dbCurrentBlockID() {
		t.Error("cstMain and cstBackup do not share the same path")
	}
	if cstBackup.cs.dbConsensusChecksum() != cstMain.cs.dbConsensusChecksum() {
		t.Error("cstMain and cstBackup do not share a consensus set hash")
	}

	// Mine 3 blocks on cstAlt, then all the block types, to give it a heavier
	// weight, then give all of its blocks to cstMain. This will cause a complex
	// fork to happen.
	for i := 0; i < 3; i++ {
		block, _ := cstAlt.miner.FindBlock()
		err = cstAlt.cs.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}
	err = cstAlt.complexBlockSet()
	if err != nil {
		t.Error(err)
	}
	var cstAltBlocks []types.Block
	pb = cstAlt.cs.dbCurrentProcessedBlock()
	for pb.Block.ID() != cstAlt.cs.blockRoot.Block.ID() {
		cstAltBlocks = append([]types.Block{pb.Block}, cstAltBlocks...) // prepend
		pb, err = cstAlt.cs.dbGetBlockMap(pb.Block.ParentID)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, block := range cstAltBlocks {
		// Some blocks will return errors.
		_ = cstMain.cs.AcceptBlock(block)
	}
	if cstMain.cs.dbCurrentBlockID() != cstAlt.cs.dbCurrentBlockID() {
		t.Error("cstMain and cstAlt do not share the same path")
	}
	if cstMain.cs.dbConsensusChecksum() != cstAlt.cs.dbConsensusChecksum() {
		t.Error("cstMain and cstAlt do not share the same consensus set hash")
	}

	// Mine 6 blocks on cstBackup and then give those blocks to cstMain, which will
	// cause cstMain to switch back to its old chain. cstMain will then have created,
	// reverted, and reapplied all the significant types of blocks.
	for i := 0; i < 6; i++ {
		block, _ := cstBackup.miner.FindBlock()
		err = cstBackup.cs.AcceptBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}
	var cstBackupBlocks []types.Block
	pb = cstBackup.cs.dbCurrentProcessedBlock()
	for pb.Block.ID() != cstBackup.cs.blockRoot.Block.ID() {
		cstBackupBlocks = append([]types.Block{pb.Block}, cstBackupBlocks...) // prepend
		pb, err = cstBackup.cs.dbGetBlockMap(pb.Block.ParentID)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, block := range cstBackupBlocks {
		// Some blocks will return errors.
		_ = cstMain.cs.AcceptBlock(block)
	}
	if cstMain.cs.dbCurrentBlockID() != cstBackup.cs.dbCurrentBlockID() {
		t.Error("cstMain and cstBackup do not share the same path")
	}
	if cstMain.cs.dbConsensusChecksum() != cstBackup.cs.dbConsensusChecksum() {
		t.Error("cstMain and cstBackup do not share the same consensus set hash")
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
	pb := cst.cs.dbCurrentProcessedBlock()

	// Create a bad block that builds on a parent, so that it is part of not
	// the longest fork.
	badBlock := types.Block{
		ParentID:     pb.Block.ParentID,
		Timestamp:    types.CurrentTimestamp(),
		MinerPayouts: []types.SiacoinOutput{{Value: types.CalculateCoinbase(pb.Height)}},
		Transactions: []types.Transaction{{
			SiacoinInputs: []types.SiacoinInput{{}}, // Will trigger an error on full verification but not partial verification.
		}},
	}
	parent, err := cst.cs.dbGetBlockMap(pb.Block.ParentID)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal("a bad block failed to cause an error")
	}
}
