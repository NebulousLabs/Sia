package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
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

// Close will close all of the testers in the reorgSets. Because we don't yet
// have a good way to check errors on a deferred statement, a panic will be
// thrown if there are any problems closing the reorgSets.
func (rs *reorgSets) Close() error {
	err := rs.cstMain.Close()
	if err != nil {
		panic(err)
	}
	err = rs.cstAlt.Close()
	if err != nil {
		panic(err)
	}
	err = rs.cstBackup.Close()
	if err != nil {
		panic(err)
	}
	return nil
}

// createReorgSets creates a reorg set that is ready to be manipulated.
func createReorgSets(name string) *reorgSets {
	cstMain, err := createConsensusSetTester(name + " - 1")
	if err != nil {
		panic(err)
	}
	cstAlt, err := createConsensusSetTester(name + " - 2")
	if err != nil {
		panic(err)
	}
	cstBackup, err := createConsensusSetTester(name + " - 3")
	if err != nil {
		panic(err)
	}

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
	t.Parallel()
	rs := createReorgSets(t.Name())
	defer rs.Close()

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
	t.Parallel()
	rs := createReorgSets(t.Name())
	defer rs.Close()

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
	t.Parallel()
	rs := createReorgSets(t.Name())
	defer rs.Close()

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
	t.Parallel()
	rs := createReorgSets(t.Name())
	defer rs.Close()

	// Give a series of blocks containing a file contract and a valid storage
	// proof to cstMain.
	rs.cstMain.testMissedStorageProofBlocks()

	// Try to trigger consensus inconsistencies by doing a full reorg on the
	// simple block.
	rs.fullReorg()
}

// TestIntegrationFileContractRevisionReorg tries to reorganize a valid storage
// proof block out of, and then back into, the consensus set.
func TestIntegrationFileContractRevisionReorg(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	rs := createReorgSets(t.Name())
	defer rs.Close()

	// Give a series of blocks containing a file contract and a valid storage
	// proof to cstMain.
	rs.cstMain.testFileContractRevision()

	// Try to trigger consensus inconsistencies by doing a full reorg on the
	// simple block.
	rs.fullReorg()
}

// TestIntegrationComplexReorg stacks up blocks of all types into a single
// blockchain that undergoes a massive reorg as a stress test to the codebase.
func TestIntegrationComplexReorg(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	rs := createReorgSets(t.Name())
	defer rs.Close()

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

// TestBuriedBadFork creates a block with an invalid transaction that's not on
// the longest fork. The consensus set will not validate that block. Then valid
// blocks are added on top of it to make it the longest fork. When it becomes
// the longest fork, all the blocks should be fully validated and thrown out
// because a parent is invalid.
func TestBuriedBadFork(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
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
