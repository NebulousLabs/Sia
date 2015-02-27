package merkletree

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// TODO: 15_13, 15_10, 15_4

// TestTreeProve manually builds storage proves for trees and indexes, and
// compares the result obtained from using the TreeProve.
func TestTreeProve(t *testing.T) {
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

	tree := NewTreeProve(sha256.New(), 0)
	for i := 0; i < 0; i++ {
		tree.Push(data[i])
	}
	_, tree0 := tree.Prove()
	if len(tree0) != len(prove0) {
		t.Error("tree0 proof failed")
	}
	for i := range prove0 {
		if bytes.Compare(tree0[i], prove0[i]) != 0 {
			t.Error("tree0 proof failed")
		}
	}

	tree = NewTreeProve(sha256.New(), 0)
	for i := 0; i < 1; i++ {
		tree.Push(data[i])
	}
	_, tree1 := tree.Prove()
	if len(tree1) != len(prove1) {
		t.Error("tree1 proof failed")
	}
	for i := range prove1 {
		if bytes.Compare(tree1[i], prove1[i]) != 0 {
			t.Error("tree1 proof failed")
		}
	}

	tree = NewTreeProve(sha256.New(), 0)
	for i := 0; i < 2; i++ {
		tree.Push(data[i])
	}
	_, tree2_0 := tree.Prove()
	if len(tree2_0) != len(prove2_0) {
		t.Error("tree2_0 proof failed")
	}
	for i := range prove2_0 {
		if bytes.Compare(tree2_0[i], prove2_0[i]) != 0 {
			t.Error("tree2_0 proof failed at index", i)
			t.Error(tree2_0[i])
			t.Error(prove2_0[i])
		}
	}

	tree = NewTreeProve(sha256.New(), 1)
	for i := 0; i < 2; i++ {
		tree.Push(data[i])
	}
	_, tree2_1 := tree.Prove()
	if len(tree2_1) != len(prove2_1) {
		t.Error("tree2_1 proof failed")
	}
	for i := range prove2_1 {
		if bytes.Compare(tree2_1[i], prove2_1[i]) != 0 {
			t.Error("tree2_1 proof failed at index", i)
		}
	}

	tree = NewTreeProve(sha256.New(), 4)
	for i := 0; i < 5; i++ {
		tree.Push(data[i])
	}
	_, tree5_4 := tree.Prove()
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

	tree = NewTreeProve(sha256.New(), 5)
	for i := 0; i < 7; i++ {
		tree.Push(data[i])
	}
	_, tree7_5 := tree.Prove()
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
