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
func TestTree(t *testing.T) {
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

	for i := 0; i < 7; i++ {
		tree.Push(data[i])
	}
	tree7 := tree.Root()
	if bytes.Compare(root7, tree7) != 0 {
		t.Error("tree7 doesn't match root7")
	}

	for i := 0; i < 6; i++ {
		tree.Push(data[i])
	}
	tree6 := tree.Root()
	if bytes.Compare(root6, tree6) != 0 {
		t.Error("tree6 doesn't match root6")
	}

	for i := 0; i < 5; i++ {
		tree.Push(data[i])
	}
	tree5 := tree.Root()
	if bytes.Compare(root5, tree5) != 0 {
		t.Error("tree5 doesn't match root5")
	}

	for i := 0; i < 4; i++ {
		tree.Push(data[i])
	}
	tree4 := tree.Root()
	if bytes.Compare(root4, tree4) != 0 {
		t.Error("tree4 doesn't match root4")
	}

	for i := 0; i < 3; i++ {
		tree.Push(data[i])
	}
	tree3 := tree.Root()
	if bytes.Compare(root3, tree3) != 0 {
		t.Error("tree3 doesn't match root3")
	}

	for i := 0; i < 2; i++ {
		tree.Push(data[i])
	}
	tree2 := tree.Root()
	if bytes.Compare(root2, tree2) != 0 {
		t.Error("tree2 doesn't match root2")
	}

	for i := 0; i < 1; i++ {
		tree.Push(data[i])
	}
	tree1 := tree.Root()
	if bytes.Compare(root1, tree1) != 0 {
		t.Error("tree1 doesn't match root1")
	}

	for i := 0; i < 0; i++ {
		tree.Push(data[i])
	}
	tree0 := tree.Root()
	if bytes.Compare(root0, tree0) != 0 {
		t.Error("tree0 doesn't match root0")
	}
}
