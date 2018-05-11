package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestBacktrackToCurrentPath probes the backtrackToCurrentPath method of the
// consensus set.
func TestBacktrackToCurrentPath(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	b := cst.cs.dbCurrentProcessedBlock()

	// Backtrack from the current node to the blockchain.
	nodes := cst.cs.dbBacktrackToCurrentPath(b)
	if len(nodes) != 1 {
		t.Fatal("backtracking to the current node gave incorrect result")
	}
	if nodes[0].Block.ID() != b.ID() {
		t.Error("backtrack returned the wrong node")
	}

	// Backtrack from a node that has diverted from the current blockchain.
	child0, _ := cst.miner.FindBlock()
	child1, _ := cst.miner.FindBlock() // Is the block not on hte current path.
	err = cst.cs.AcceptBlock(child0)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.cs.AcceptBlock(child1)
	if err != modules.ErrNonExtendingBlock {
		t.Fatal(err)
	}
	b, err = cst.cs.dbGetBlockMap(child1.ID())
	if err != nil {
		t.Fatal(err)
	}
	nodes = cst.cs.dbBacktrackToCurrentPath(b)
	if len(nodes) != 2 {
		t.Error("backtracking grabbed wrong number of nodes")
	}
	parent, err := cst.cs.dbGetBlockMap(b.ParentID)
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].Block.ID() != parent.Block.ID() {
		t.Error("grabbed the wrong block as the common block")
	}
	if nodes[1].Block.ID() != b.ID() {
		t.Error("backtracked from the wrong node")
	}
}

// TestRevertToNode probes the revertToBlock method of the consensus set.
func TestRevertToNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	b := cst.cs.dbCurrentProcessedBlock()

	// Revert to a grandparent and verify the returned array is correct.
	parent, err := cst.cs.dbGetBlockMap(b.ParentID)
	if err != nil {
		t.Fatal(err)
	}
	grandParent, err := cst.cs.dbGetBlockMap(parent.Block.ParentID)
	if err != nil {
		t.Fatal(err)
	}
	revertedNodes := cst.cs.dbRevertToNode(grandParent)
	if len(revertedNodes) != 2 {
		t.Error("wrong number of nodes reverted")
	}
	if revertedNodes[0].Block.ID() != b.ID() {
		t.Error("wrong composition of reverted nodes")
	}
	if revertedNodes[1].Block.ID() != parent.Block.ID() {
		t.Error("wrong composition of reverted nodes")
	}

	// Trigger a panic by trying to revert to a node outside of the current
	// path.
	defer func() {
		r := recover()
		if r != errExternalRevert {
			t.Error(r)
		}
	}()
	cst.cs.dbRevertToNode(b)
}
