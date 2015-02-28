package merkletree

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// join takes two byte slices, appends them, hashes them, and then returns the
// result.
func join(a, b []byte) []byte {
	return sum(sha256.New(), append(a, b...))
}

// TestTree builds a few trees manually and then compares them to the result
// obtained from using the tree.
func TestBuildRoot(t *testing.T) {
	t.Skip()

	// Create the data that is going to be hashed.
	var data [][]byte
	for i := byte(0); i < 8; i++ {
		data = append(data, []byte{i})
	}

	// Join joins hashes, but the data hasn't been hashed yet because the tree
	// hashes it automatically. This prepares the data to be joined manually.
	var leaves [][]byte
	for i := byte(0); i < 8; i++ {
		leaves = append(leaves, sum(sha256.New(), data[i]))
	}

	// Build out the expected values for merkle trees of each size 0 - 8.
	root8 := join(
		join(
			join(leaves[0], leaves[1]),
			join(leaves[2], leaves[3]),
		),
		join(
			join(leaves[4], leaves[5]),
			join(leaves[6], leaves[7]),
		),
	)

	root7 := join(
		join(
			join(leaves[0], leaves[1]),
			join(leaves[2], leaves[3]),
		),
		join(
			join(leaves[4], leaves[5]),
			leaves[6],
		),
	)

	root6 := join(
		join(
			join(leaves[0], leaves[1]),
			join(leaves[2], leaves[3]),
		),
		join(
			leaves[4],
			leaves[5],
		),
	)

	root5 := join(
		join(
			join(leaves[0], leaves[1]),
			join(leaves[2], leaves[3]),
		),
		leaves[4],
	)

	root4 := join(
		join(leaves[0], leaves[1]),
		join(leaves[2], leaves[3]),
	)

	root3 := join(
		join(leaves[0], leaves[1]),
		leaves[2],
	)
	root2 := join(leaves[0], leaves[1])

	root1 := leaves[0]

	root0 := []byte(nil)

	// Create the tree.
	tree := New(sha256.New())

	// Try building the trees for sizes 0 through 8 and see if it matches the
	// manually obtained Merkle roots.
	for i := 0; i < 8; i++ {
		tree.Push(data[i])
	}
	tree8 := tree.Root()
	if bytes.Compare(root8, tree8) != 0 {
		t.Error("tree8 doesn't match root8")
	}

	tree.Reset()
	for i := 0; i < 7; i++ {
		tree.Push(data[i])
	}
	tree7 := tree.Root()
	if bytes.Compare(root7, tree7) != 0 {
		t.Error("tree7 doesn't match root7")
	}

	tree.Reset()
	for i := 0; i < 6; i++ {
		tree.Push(data[i])
	}
	tree6 := tree.Root()
	if bytes.Compare(root6, tree6) != 0 {
		t.Error("tree6 doesn't match root6")
	}

	tree.Reset()
	for i := 0; i < 5; i++ {
		tree.Push(data[i])
	}
	tree5 := tree.Root()
	if bytes.Compare(root5, tree5) != 0 {
		t.Error("tree5 doesn't match root5")
	}

	tree.Reset()
	for i := 0; i < 4; i++ {
		tree.Push(data[i])
	}
	tree4 := tree.Root()
	if bytes.Compare(root4, tree4) != 0 {
		t.Error("tree4 doesn't match root4")
	}

	tree.Reset()
	for i := 0; i < 3; i++ {
		tree.Push(data[i])
	}
	tree3 := tree.Root()
	if bytes.Compare(root3, tree3) != 0 {
		t.Error("tree3 doesn't match root3")
	}

	tree.Reset()
	for i := 0; i < 2; i++ {
		tree.Push(data[i])
	}
	tree2 := tree.Root()
	if bytes.Compare(root2, tree2) != 0 {
		t.Error("tree2 doesn't match root2")
	}

	tree.Reset()
	for i := 0; i < 1; i++ {
		tree.Push(data[i])
	}
	tree1 := tree.Root()
	if bytes.Compare(root1, tree1) != 0 {
		t.Error("tree1 doesn't match root1")
	}

	tree.Reset()
	for i := 0; i < 0; i++ {
		tree.Push(data[i])
	}
	tree0 := tree.Root()
	if bytes.Compare(root0, tree0) != 0 {
		t.Error("tree0 doesn't match root0")
	}
}

// TestTreeProve manually builds storage proves for trees and indexes, and
// compares the result obtained from using the TreeProve.
//
// TODO: 15_13, 15_10, 15_4
func TestBuildProof(t *testing.T) {
	// Create the data that is going to be hashed.
	var data [][]byte
	for i := byte(0); i < 8; i++ {
		data = append(data, []byte{i})
	}

	// Join joins hashes, but the data hasn't been hashed yet because the tree
	// hashes it automatically. This prepares the data to be joined manually.
	var leaves [][]byte
	for i := byte(0); i < 8; i++ {
		leaves = append(leaves, sum(sha256.New(), data[i]))
	}

	// Manually create proofs for chosen set of edge cases.
	prove0 := [][]byte(nil)

	var prove1 [][]byte
	prove1 = append(prove1, data[0])

	var prove2_0 [][]byte
	prove2_0 = append(prove2_0, data[0])
	prove2_0 = append(prove2_0, leaves[1])

	var prove2_1 [][]byte
	prove2_1 = append(prove2_1, data[1])
	prove2_1 = append(prove2_1, leaves[0])

	var prove5_4 [][]byte
	prove5_4 = append(prove5_4, data[4])
	prove5_4 = append(prove5_4, join(
		join(leaves[0], leaves[1]),
		join(leaves[2], leaves[3]),
	))

	var prove7_5 [][]byte
	prove7_5 = append(prove7_5, data[5])
	prove7_5 = append(prove7_5, leaves[4])
	prove7_5 = append(prove7_5, leaves[6])
	prove7_5 = append(prove7_5, join(
		join(leaves[0], leaves[1]),
		join(leaves[2], leaves[3]),
	))

	// Create a proof using the tree for each of the manually built proofs and
	// verify that the results are identical.
	tree := New(sha256.New())
	for i := 0; i < 0; i++ {
		tree.Push(data[i])
	}
	tree0 := tree.Prove()
	if len(tree0) != len(prove0) {
		t.Error("tree0 proof failed")
	}
	for i := range prove0 {
		if bytes.Compare(tree0[i], prove0[i]) != 0 {
			t.Error("tree0 proof failed")
		}
	}

	tree.SetIndex(0)
	for i := 0; i < 1; i++ {
		tree.Push(data[i])
	}
	tree1 := tree.Prove()
	if len(tree1) != len(prove1) {
		t.Error(len(tree1))
		t.Error(len(prove1))
		t.Fatal("tree1 proof failed")
	}
	for i := range prove1 {
		if bytes.Compare(tree1[i], prove1[i]) != 0 {
			t.Error("tree1 proof failed")
		}
	}

	tree.SetIndex(0)
	for i := 0; i < 2; i++ {
		tree.Push(data[i])
	}
	tree2_0 := tree.Prove()
	if len(tree2_0) != len(prove2_0) {
		t.Error(len(tree2_0))
		t.Error(len(prove2_0))
		t.Error("tree2_0 proof failed")
	}
	for i := range prove2_0 {
		if bytes.Compare(tree2_0[i], prove2_0[i]) != 0 {
			t.Error("tree2_0 proof failed at index", i)
			t.Error(tree2_0[i])
			t.Error(prove2_0[i])
		}
	}

	tree.SetIndex(1)
	for i := 0; i < 2; i++ {
		tree.Push(data[i])
	}
	tree2_1 := tree.Prove()
	if len(tree2_1) != len(prove2_1) {
		t.Error("tree2_1 proof failed")
	}
	for i := range prove2_1 {
		if bytes.Compare(tree2_1[i], prove2_1[i]) != 0 {
			t.Error("tree2_1 proof failed at index", i)
			t.Error(tree2_1[i])
			t.Error(prove2_1[i])
		}
	}

	tree.SetIndex(4)
	for i := 0; i < 5; i++ {
		tree.Push(data[i])
	}
	tree5_4 := tree.Prove()
	if len(tree5_4) != len(prove5_4) {
		t.Error(len(tree5_4))
		t.Error(len(prove5_4))
		t.Fatal("tree5_4 proof failed")
	}
	for i := range prove5_4 {
		if bytes.Compare(tree5_4[i], prove5_4[i]) != 0 {
			t.Error("tree5_4 proof failed at index", i)
			t.Error(tree5_4[i])
		}
	}

	tree.SetIndex(5)
	for i := 0; i < 7; i++ {
		tree.Push(data[i])
	}
	tree7_5 := tree.Prove()
	if len(tree7_5) != len(prove7_5) {
		t.Error(len(tree7_5))
		t.Error(len(prove7_5))
		t.Fatal("tree7_5 proof failed")
	}
	for i := range prove7_5 {
		if bytes.Compare(tree7_5[i], prove7_5[i]) != 0 {
			t.Error("tree7_5 proof failed at index", i)
			t.Error(tree7_5[i])
			t.Error(prove7_5[i])
		}
	}
}
