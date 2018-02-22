package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/node"
	"github.com/NebulousLabs/Sia/siatest"
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
