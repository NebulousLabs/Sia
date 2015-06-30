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
	bn := cst.cs.currentBlockNode()

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
	cst.csUpdateWait()
	grandchild, _ := cst.miner.FindBlock()
	err = cst.cs.AcceptBlock(grandchild)
	if err != nil {
		t.Fatal(err)
	}
	cst.csUpdateWait()
	err = cst.cs.AcceptBlock(child1)
	if err != modules.ErrNonExtendingBlock {
		t.Fatal(err)
	}

	// Check the structure is as intended.
	if len(bn.children) != 2 {
		t.Fatal("wrong number of children on parent block")
	}
	if len(bn.children[0].children) != 1 {
		t.Fatal("bad block doesn't have the right number of children")
	}
	if len(bn.children[1].children) != 0 {
		t.Fatal("good block has children")
	}

	// Rewind so that 'bn' is the current block again.
	cst.cs.commitDiffSet(bn.children[0].children[0], modules.DiffRevert)
	cst.cs.commitDiffSet(bn.children[0], modules.DiffRevert)

	// Call 'deleteNode' on child0
	child0Node := bn.children[0]
	cst.cs.deleteNode(bn.children[0])
	if len(bn.children) != 1 {
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
