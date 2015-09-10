package consensus

import (
	"testing"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/modules"
)

// dbBacktrackToCurrentPath is a convenience function to call
// backtrackToCurrentPath without a bolt.Tx.
func (cs *ConsensusSet) dbBacktrackToCurrentPath(pb *processedBlock) (pbs []*processedBlock) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		pbs = backtrackToCurrentPath(tx, pb)
		return nil
	})
	return pbs
}

// dbRevertToNode is a convenience function to call revertToBlock without a
// bolt.Tx.
func (cs *ConsensusSet) dbRevertToNode(pb *processedBlock) (pbs []*processedBlock) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		pbs = revertToBlock(tx, pb)
		return nil
	})
	return pbs
}

// TestBacktrackToCurrentPath probes the backtrackToCurrentPath method of the
// consensus set.
func TestBacktrackToCurrentPath(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestBacktrackToCurrentPath")
	if err != nil {
		t.Fatal(err)
	}
	pb := cst.cs.currentProcessedBlock()

	// Backtrack from the current node to the blockchain.
	nodes := cst.cs.dbBacktrackToCurrentPath(pb)
	if len(nodes) != 1 {
		t.Fatal("backtracking to the current node gave incorrect result")
	}
	if nodes[0].Block.ID() != pb.Block.ID() {
		t.Error("backtrack returned the wrong node")
	}

	// Backtrack from a node that has diverted from the current blockchain.
	child0, _ := cst.miner.FindBlock()
	child1, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(child0)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.cs.AcceptBlock(child1)
	if err != modules.ErrNonExtendingBlock {
		t.Fatal(err)
	}
	pb = cst.cs.db.getBlockMap(child1.ID())
	nodes = cst.cs.dbBacktrackToCurrentPath(pb)
	if len(nodes) != 2 {
		t.Error("backtracking grabbed wrong number of nodes")
	}
	parent := cst.cs.db.getBlockMap(pb.Parent)
	if nodes[0].Block.ID() != parent.Block.ID() {
		t.Error("grabbed the wrong block as the common block")
	}
	if nodes[1].Block.ID() != pb.Block.ID() {
		t.Error("backtracked from the wrong node")
	}
}

// TestRevertToNode probes the revertToBlock method of the consensus set.
func TestRevertToNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestBacktrackToCurrentPath")
	if err != nil {
		t.Fatal(err)
	}
	pb := cst.cs.currentProcessedBlock()

	// Revert to a grandparent and verify the returned array is correct.
	parent := cst.cs.db.getBlockMap(pb.Parent)
	grandParent := cst.cs.db.getBlockMap(parent.Parent)
	revertedNodes := cst.cs.dbRevertToNode(grandParent)
	if len(revertedNodes) != 2 {
		t.Error("wrong number of nodes reverted")
	}
	if revertedNodes[0].Block.ID() != pb.Block.ID() {
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
	cst.cs.dbRevertToNode(pb)
}
