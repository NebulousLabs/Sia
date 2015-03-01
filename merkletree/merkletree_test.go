package merkletree

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// A MerkleTester contains data types which are used in multiple tests.
type MerkleTester struct {
	data      [][]byte
	leaves    [][]byte
	roots     map[int][]byte
	proveSets map[int]map[int][][]byte
	*testing.T
}

// join returns the sha256 hash of a+b.
func (mt *MerkleTester) join(a, b []byte) []byte {
	return sum(sha256.New(), append(a, b...))
}

// CreateMerkleTester creates a merkle tester and manually fills out many of
// the expected values for constructing merkle tree roots and merkle tree
// proofs. These manual values can then be compared against the values that the
// Tree creates.
func CreateMerkleTester(t *testing.T) (mt *MerkleTester) {
	mt = &MerkleTester{
		roots:     make(map[int][]byte),
		proveSets: make(map[int]map[int][][]byte),
	}
	mt.T = t

	// Fill out the data and leaves values.
	size := 16
	for i := 0; i < size; i++ {
		mt.data = append(mt.data, []byte{byte(i)})
	}
	for i := 0; i < size; i++ {
		mt.leaves = append(mt.leaves, sum(sha256.New(), mt.data[i]))
	}

	// Manually build out expected merkle root values.
	mt.roots[0] = []byte(nil)
	mt.roots[1] = mt.leaves[0]
	mt.roots[2] = mt.join(mt.leaves[0], mt.leaves[1])
	mt.roots[3] = mt.join(
		mt.roots[2],
		mt.leaves[2],
	)
	mt.roots[4] = mt.join(
		mt.roots[2],
		mt.join(mt.leaves[2], mt.leaves[3]),
	)
	mt.roots[5] = mt.join(
		mt.roots[4],
		mt.leaves[4],
	)

	mt.roots[6] = mt.join(
		mt.roots[4],
		mt.join(
			mt.leaves[4],
			mt.leaves[5],
		),
	)

	mt.roots[7] = mt.join(
		mt.roots[4],
		mt.join(
			mt.join(mt.leaves[4], mt.leaves[5]),
			mt.leaves[6],
		),
	)

	mt.roots[8] = mt.join(
		mt.roots[4],
		mt.join(
			mt.join(mt.leaves[4], mt.leaves[5]),
			mt.join(mt.leaves[6], mt.leaves[7]),
		),
	)

	mt.roots[15] = mt.join(
		mt.roots[8],
		mt.join(
			mt.join(
				mt.join(mt.leaves[8], mt.leaves[9]),
				mt.join(mt.leaves[10], mt.leaves[11]),
			),
			mt.join(
				mt.join(mt.leaves[12], mt.leaves[13]),
				mt.leaves[14],
			),
		),
	)

	// Manually build out some prove sets that should should match what the
	// Tree creates for the same values.
	mt.proveSets[0] = make(map[int][][]byte)
	mt.proveSets[0][0] = [][]byte(nil)

	mt.proveSets[1] = make(map[int][][]byte)
	mt.proveSets[1][0] = append([][]byte(nil), mt.data[0])

	mt.proveSets[2] = make(map[int][][]byte)
	mt.proveSets[2][0] = append(mt.proveSets[2][0], mt.data[0])
	mt.proveSets[2][0] = append(mt.proveSets[2][0], mt.leaves[1])

	mt.proveSets[2][1] = append(mt.proveSets[2][1], mt.data[1])
	mt.proveSets[2][1] = append(mt.proveSets[2][1], mt.leaves[0])

	mt.proveSets[5] = make(map[int][][]byte)
	mt.proveSets[5][4] = append(mt.proveSets[5][4], mt.data[4])
	mt.proveSets[5][4] = append(mt.proveSets[5][4], mt.roots[4])

	mt.proveSets[7] = make(map[int][][]byte)
	mt.proveSets[7][5] = append(mt.proveSets[7][5], mt.data[5])
	mt.proveSets[7][5] = append(mt.proveSets[7][5], mt.leaves[4])
	mt.proveSets[7][5] = append(mt.proveSets[7][5], mt.leaves[6])
	mt.proveSets[7][5] = append(mt.proveSets[7][5], mt.roots[4])

	mt.proveSets[15] = make(map[int][][]byte)
	mt.proveSets[15][10] = append(mt.proveSets[15][10], mt.data[10])
	mt.proveSets[15][10] = append(mt.proveSets[15][10], mt.leaves[11])
	mt.proveSets[15][10] = append(mt.proveSets[15][10], mt.join(
		mt.leaves[8],
		mt.leaves[9],
	))
	mt.proveSets[15][10] = append(mt.proveSets[15][10], mt.join(
		mt.join(mt.leaves[12], mt.leaves[13]),
		mt.leaves[14],
	))
	mt.proveSets[15][10] = append(mt.proveSets[15][10], mt.roots[8])

	mt.proveSets[15][3] = append(mt.proveSets[15][3], mt.data[3])
	mt.proveSets[15][3] = append(mt.proveSets[15][3], mt.leaves[2])
	mt.proveSets[15][3] = append(mt.proveSets[15][3], mt.roots[2])
	mt.proveSets[15][3] = append(mt.proveSets[15][3], mt.join(
		mt.join(mt.leaves[4], mt.leaves[5]),
		mt.join(mt.leaves[6], mt.leaves[7]),
	))
	mt.proveSets[15][3] = append(mt.proveSets[15][3], mt.join(
		mt.join(
			mt.join(mt.leaves[8], mt.leaves[9]),
			mt.join(mt.leaves[10], mt.leaves[11]),
		),
		mt.join(
			mt.join(mt.leaves[12], mt.leaves[13]),
			mt.leaves[14],
		),
	))

	return
}

// TestTree builds a few trees manually and then compares them to the result
// obtained from using the tree.
func TestBuildRoot(t *testing.T) {
	mt := CreateMerkleTester(t)

	// Compare the results of calling Root against all of the manually
	// constructed merkle trees.
	tree := New(sha256.New())
	for i, root := range mt.roots {
		// Fill out the tree.
		tree.Reset()
		for j := 0; j < i; j++ {
			tree.Push(mt.data[j])
		}

		// Get the root and compare to the manually constructed root.
		treeRoot := tree.Root()
		if bytes.Compare(root, treeRoot) != 0 {
			t.Error("tree root doesn't match manual root for index", i)
		}

		// Check that calling Root a second time results in the same
		// value.
		if bytes.Compare(treeRoot, tree.Root()) != 0 {
			t.Error("consecutive calls to Root did not produce the same value for index", i)
		}
	}
}

// TestTreeProve manually builds storage proves for trees and indexes, and
// compares the result obtained from using the TreeProve.
func TestBuildAndVerifyProof(t *testing.T) {
	mt := CreateMerkleTester(t)

	// Compare the results of building a merkle proof to all of the manually
	// constructed proofs.
	tree := New(sha256.New())
	for i, manualProveSets := range mt.proveSets {
		for j, expectedProveSet := range manualProveSets {
			// Build out the tree.
			tree.SetIndex(j)
			for k := 0; k < i; k++ {
				tree.Push(mt.data[k])
			}

			// Get the proof and check all values.
			hash, merkleRoot, proveSet, proveIndex, numSegments := tree.Prove()
			if bytes.Compare(merkleRoot, mt.roots[i]) != 0 {
				t.Error("incorrect Merkle root returned by Tree for indices", i, j)
			}
			if len(proveSet) != len(expectedProveSet) {
				t.Error("prove set is wrong lenght for indices", i, j)
				continue
			}
			if proveIndex != j {
				t.Error("incorrect proveIndex returned for indices", i, j)
			}
			if numSegments != i {
				t.Error("incorrect numSegments returned for indices", i, j)
			}
			for k := range proveSet {
				if bytes.Compare(proveSet[k], expectedProveSet[k]) != 0 {
					t.Error("prove set does not match expected prove set for indices", i, j, k)
				}
			}

			// Check that verification works on for the desired prove index but
			// fails for all other indices.
			if !VerifyProof(hash, merkleRoot, proveSet, proveIndex, numSegments) {
				t.Error("prove set does not verify!")
			}
			for k := 0; k < i; k++ {
				if k == proveIndex {
					continue
				}
				if VerifyProof(hash, merkleRoot, proveSet, k, numSegments) {
					t.Error("prove set verifies for wrong index at indices", i, j, k)
				}
			}

			// Check that calling Prove a second time results in the same
			// values.
			hash2, merkleRoot2, proveSet2, proveIndex2, numSegments2 := tree.Prove()
			if hash2 != hash {
				t.Error("tree returned different hashes after calling Prove twice for indices", i, j)
			}
			if bytes.Compare(merkleRoot, merkleRoot2) != 0 {
				t.Error("tree returned different merkle roots after calling Prove twice for indices", i, j)
			}
			if len(proveSet) != len(proveSet2) {
				t.Error("tree returned different prove sets after calling Prove twice for indices", i, j)
			}
			for k := range proveSet {
				if bytes.Compare(proveSet[k], proveSet2[k]) != 0 {
					t.Error("tree returned different prove sets after calling Prove twice for indices", i, j)
				}
			}
			if proveIndex != proveIndex2 {
				t.Error("tree returned different prove indexes after calling Prove twice for indices", i, j)
			}
			if numSegments != numSegments2 {
				t.Error("tree returned different segment count after calling Prove twice for indices", i, j)
			}
		}
	}
}
