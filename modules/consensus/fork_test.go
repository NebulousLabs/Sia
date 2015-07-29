package consensus

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestDeleteNode probes the deleteNode method of the consensus set.
func TestDeleteNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestDeleteNode")
	if err != nil {
		t.Fatal(err)
	}
	pb := cst.cs.currentProcessedBlock()

	// Set up the following structure:
	//		parent -> child0 + child1
	//		child0 -> grandchild
	//		child1 -> nil
	//		grandchild -> nil
	//
	// When child0 is removed from the list, the following structure should
	// remain:
	//		parent -> child1Good
	//		child1Good -> nil
	child0, _ := cst.miner.FindBlock()
	child1, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(child0)
	if err != nil {
		t.Fatal(err)
	}
	grandchild, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(grandchild)
	if err != nil {
		t.Fatal(err)
	}
	err = cst.cs.AcceptBlock(child1)
	if err != modules.ErrNonExtendingBlock {
		t.Fatal(err)
	}

	// Check the structure is as intended.
	if len(pb.Children) != 2 {
		t.Fatal("wrong number of children on parent block")
	}
	pbChild0 := cst.cs.db.getBlockMap(pb.Children[0])
	if len(pbChild0.Children) != 1 {
		t.Fatal("bad block doesn't have the right number of children")
	}
	pbChild1 := cst.cs.db.getBlockMap(pb.Children[1])
	if len(pbChild1.Children) != 0 {
		t.Fatal("good block has children")
	}

	// Rewind so that 'pb' is the current block again.
	childchild := cst.cs.db.getBlockMap(child0.Children[0])
	cst.cs.commitDiffSet(childchild, modules.DiffRevert)
	cst.cs.commitDiffSet(child, modules.DiffRevert)

	// Call 'deleteNode' on child0
	child0Node := cst.cs.db.getBlockMap(pb.Children[0])
	cst.cs.deleteNode(child0Node)
	if len(pb.Children) != 1 {
		t.Error("children not correctly deleted")
	}
	if len(child0Node.children) != 0 {
		t.Error("grandchild not deleted correctly")
	}
	if bn.children[0] == child0Node {
		t.Error("wrong child was deleted")
	}

	// Trigger a panic by calling 'deleteNode' on a block node in the current
	// path.
	defer func() {
		r := recover()
		if r != errDeleteCurrentPath {
			t.Error("expecting errDeleteCurrentPath, got", r)
		}
	}()
	cst.cs.deleteNode(bn)
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
	bn := cst.cs.currentBlockNode()

	// Backtrack from the current node to the blockchain.
	nodes := cst.cs.backtrackToCurrentPath(bn)
	if len(nodes) != 1 {
		t.Fatal("backtracking to the current node gave incorrect result")
	}
	if nodes[0].block.ID() != bn.block.ID() {
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
	bn = cst.cs.blockMap[child1.ID()]
	nodes = cst.cs.backtrackToCurrentPath(bn)
	if len(nodes) != 2 {
		t.Error("backtracking grabbed wrong number of nodes")
	}
	if nodes[0].block.ID() != bn.parent.block.ID() {
		t.Error("grabbed the wrong block as the common block")
	}
	if nodes[1].block.ID() != bn.block.ID() {
		t.Error("backtracked from the wrong node")
	}
}

// TestRevertToNode probes the revertToNode method of the consensus set.
func TestRevertToNode(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester("TestBacktrackToCurrentPath")
	if err != nil {
		t.Fatal(err)
	}
	bn := cst.cs.currentBlockNode()

	// Revert to a grandparent and verify the returned array is correct.
	revertedNodes := cst.cs.revertToNode(bn.parent.parent)
	if len(revertedNodes) != 2 {
		t.Error("wrong number of nodes reverted")
	}
	if revertedNodes[0] != bn {
		t.Error("wrong composition of reverted nodes")
	}
	if revertedNodes[1] != bn.parent {
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
	cst.cs.revertToNode(bn)
}
