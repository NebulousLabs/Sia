package consensus

import (
	"bytes"
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

// TestConsensusHeadersHeightGet tests the /consensus/headers/:height
// endpoint
func TestConsensusHeadersHeightGet(t *testing.T) {
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

	// Get current block id using the /consensus endpoint
	testNode.MineBlock()
	cg, err := testNode.ConsensusGet()
	if err != nil {
		t.Fatal(err)
	}
	// Get the current block id using the /consensus/headers/height endpoint
	// and compare it to the other value
	chg, err := testNode.ConsensusHeadersHeightGet(cg.Height)
	if err != nil {
		t.Fatal("Failed to get header information", err)
	}
	if bytes.Compare(chg.BlockID[:], cg.CurrentBlock[:]) != 0 {
		t.Fatal("BlockIDs do not match")
	}
}

// TestConsensusBlocksIDGet tests the /consensus/blocks/:id endpoint
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

	// Send GET request
	cg, err := testNode.ConsensusGet()
	if err != nil {
		t.Fatal(err)
	}
	block, err := testNode.ConsensusBlocksIDGet(cg.CurrentBlock)
	if err != nil {
		t.Fatal("Failed to retrieve block", err)
	}
	// Make sure all of the fields are initialized and not empty
	var zeroID types.BlockID
	if bytes.Compare(block.ParentID[:], zeroID[:]) == 0 {
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
}
