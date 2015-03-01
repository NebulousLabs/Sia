package merkletree

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// A MerkleTester contains data types that can be filled out manually to
// compare against function results.
type MerkleTester struct {
	// data is the raw data of the merkle tree.
	data [][]byte

	// leaves is the hashes of the data, and should be the same length.
	leaves [][]byte

	// roots contains the root hashes of merkle trees of various heights using
	// the data for input.
	roots map[int][]byte

	// proveSets contains proofs that certain data is in a merkle tree. The
	// first map is the number of leaves in the tree that the proof is for. The
	// root of that tree can be found in roots. The second map is the
	// proveIndex that was used when building the proof.
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

	return
}

// TestBuildRoot checks that the root returned by Tree matches the manually
// created roots for all of the manually created roots.
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
	}
}

// TestBuildAndVerifyProof builds a proof using a tree for every single
// manually created proof in the MerkleTester. Then it checks that the proof
// matches the manually created proof, and that the proof is verified by
// VerifyProof. Then it checks that the proof fails for all other indices,
// which should happen if all of the leaves are unique.
func TestBuildAndVerifyProof(t *testing.T) {
	mt := CreateMerkleTester(t)

	// Compare the results of building a merkle proof to all of the manually
	// constructed proofs.
	tree := New(sha256.New())
	for i, manualProveSets := range mt.proveSets {
		for j, expectedProveSet := range manualProveSets {
			// Build out the tree.
			tree.Reset()
			err := tree.SetIndex(j)
			if err != nil {
				t.Fatal(err)
			}
			for k := 0; k < i; k++ {
				tree.Push(mt.data[k])
			}

			// Get the proof and check all values.
			hash, merkleRoot, proveSet, proveIndex, numSegments := tree.Prove()
			if bytes.Compare(merkleRoot, mt.roots[i]) != 0 {
				t.Error("incorrect Merkle root returned by Tree for indices", i, j)
			}
			if len(proveSet) != len(expectedProveSet) {
				t.Error("prove set is wrong length for indices", i, j)
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
				t.Error("prove set does not verify for indices", i, j)
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

// TestBadInputs provides malicious inputs to the functions of the package,
// trying to trigger panics or unexpected behavior.
func TestBadInputs(t *testing.T) {
	// Put nil into sum.
	a := sum(sha256.New(), nil)
	if a != nil {
		t.Error("sum of nil should return nil")
	}

	// Get the root and proof of an empty tree.
	tree := New(sha256.New())
	root := tree.Root()
	if root != nil {
		t.Error("root of empty tree should be nil")
	}
	_, _, proof, _, _ := tree.Prove()
	if proof != nil {
		t.Error("proof of empty tree should be nil")
	}

	// Get the proof of a tree that hasn't reached it's index.
	err := tree.SetIndex(3)
	if err != nil {
		t.Fatal(err)
	}
	tree.Push([]byte{1})
	_, _, proof, _, _ = tree.Prove()
	if proof != nil {
		t.Fatal(err)
	}
	err = tree.SetIndex(2)
	if err == nil {
		t.Error("expecting error, shouldn't be able to reset a tree after pushing")
	}

	// Try nil values in VerifyProof.
	mt := CreateMerkleTester(t)
	if VerifyProof(sha256.New(), nil, mt.proveSets[1][0], 0, 1) {
		t.Error("VerifyProof should return false for nil merkle root")
	}
	if VerifyProof(sha256.New(), []byte{1}, nil, 0, 1) {
		t.Error("VerifyProof should return false for nil prove set")
	}
	if VerifyProof(sha256.New(), mt.roots[15], mt.proveSets[15][3][1:], 3, 15) {
		t.Error("VerifyPRoof should return false for too-short prove set")
	}
	if VerifyProof(sha256.New(), mt.roots[15], mt.proveSets[15][10][1:], 10, 15) {
		t.Error("VerifyPRoof should return false for too-short prove set")
	}
}

// TestCompatibility runs BuildProof for a large set of trees, and checks that
// verify affirms each proof, while rejecting for all other indexes (this
// second half requires that all input data be unique).
func TestCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Brute force all trees up to size 'max'. Running time for this test is max^3.
	max := 32
	tree := New(sha256.New())
	for i := 1; i < max; i++ {
		// Try with proof at every possible index.
		for j := 0; j < i; j++ {
			// Push unique data into the tree.
			tree.Reset()
			err := tree.SetIndex(j)
			if err != nil {
				t.Fatal(err)
			}
			for k := 0; k < i; k++ {
				tree.Push([]byte{byte(k)})
			}

			// Build the proof for the tree and run it through verify.
			hash, merkleRoot, proveSet, proveIndex, numLeaves := tree.Prove()
			if !VerifyProof(hash, merkleRoot, proveSet, proveIndex, numLeaves) {
				t.Error("proof didn't verify for indices", i, j)
			}

			// Check that verification fails for all other indices.
			for k := 0; k < i; k++ {
				if k == j {
					continue
				}
				if VerifyProof(hash, merkleRoot, proveSet, k, numLeaves) {
					t.Error("proof verified for indices", i, j, k)
				}
			}
		}
	}
}
