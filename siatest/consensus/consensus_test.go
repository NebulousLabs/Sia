package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/siatest"
	"github.com/NebulousLabs/Sia/types"
)

// TestApiHeight checks if the consensus api endpoint works
func TestApiHeight(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testdir, err := siatest.TestDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a new server
	testNode, err := siatest.NewNode(node.AllModules(testdir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := testNode.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Send GET request
	cg, err := testNode.ConsensusGet()
	if err != nil {
		t.Fatal(err)
	}
	height := cg.Height

	// Mine a block
	if err := testNode.MineBlock(); err != nil {
		t.Fatal(err)
	}

	// Request height again and check if it increased
	cg, err = testNode.ConsensusGet()
	if err != nil {
		t.Fatal(err)
	}
	if cg.Height != height+1 {
		t.Fatal("Height should have increased by 1 block")
	}
}

// TestConsensusBlocksIDGet tests the /consensus/blocks endpoint
func TestConsensusBlocksIDGet(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testdir, err := siatest.TestDir(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a new server
	testNode, err := siatest.NewNode(node.AllModules(testdir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := testNode.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Send /consensus request
	cg, err := testNode.ConsensusGet()
	if err != nil {
		t.Fatal(err)
	}

	// Get block by id
	block, err := testNode.ConsensusBlocksIDGet(cg.CurrentBlock)
	if err != nil {
		t.Fatal("Failed to retrieve block", err)
	}
	// Make sure all of the fields are initialized and not empty
	var zeroID types.BlockID
	if block.ID != cg.CurrentBlock {
		t.Fatal("BlockID wasn't set correctly")
	}
	if block.Height != cg.Height {
		t.Fatal("BlockHeight wasn't set correctly")
	}
	if block.ParentID == zeroID {
		t.Fatal("ParentID wasn't set correctly")
	}
	if block.Timestamp == types.Timestamp(0) {
		t.Fatal("Timestamp wasn't set correctly")
	}
	if len(block.MinerPayouts) == 0 {
		t.Fatal("Block has no miner payouts")
	}
	if len(block.Transactions) == 0 {
		t.Fatal("Block doesn't have any transactions even though it should")
	}

	// Get same block by height
	block2, err := testNode.ConsensusBlocksHeightGet(cg.Height)
	if err != nil {
		t.Fatal("Failed to retrieve block", err)
	}
	// block and block2 should be the same
	if block.ID != block2.ID {
		t.Fatal("BlockID wasn't set correctly")
	}
	if block.Height != block2.Height {
		t.Fatal("BlockID wasn't set correctly")
	}
	if block.ParentID != block2.ParentID {
		t.Fatal("ParentIDs don't match")
	}
	if block.Timestamp != block2.Timestamp {
		t.Fatal("Timestamps don't match")
	}
	if len(block.MinerPayouts) != len(block2.MinerPayouts) {
		t.Fatal("MinerPayouts don't match")
	}
	if len(block.Transactions) != len(block2.Transactions) {
		t.Fatal("Transactions don't match")
	}
}
