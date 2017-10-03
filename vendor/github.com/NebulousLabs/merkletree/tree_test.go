package merkletree

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"strconv"
	"testing"
)

// A MerkleTester contains data types that can be filled out manually to
// compare against function results.
type MerkleTester struct {
	// data is the raw data of the Merkle tree.
	data [][]byte

	// leaves is the hashes of the data, and should be the same length.
	leaves [][]byte

	// roots contains the root hashes of Merkle trees of various heights using
	// the data for input.
	roots map[int][]byte

	// proofSets contains proofs that certain data is in a Merkle tree. The
	// first map is the number of leaves in the tree that the proof is for. The
	// root of that tree can be found in roots. The second map is the
	// proofIndex that was used when building the proof.
	proofSets map[int]map[int][][]byte
	*testing.T
}

// join returns the sha256 hash of 0x01 || a || b.
func (mt *MerkleTester) join(a, b []byte) []byte {
	return sum(sha256.New(), append(append([]byte{1}, a...), b...))
}

// CreateMerkleTester creates a Merkle tester and manually fills out many of
// the expected values for constructing Merkle tree roots and Merkle tree
// proofs. These manual values can then be compared against the values that the
// Tree creates.
func CreateMerkleTester(t *testing.T) (mt *MerkleTester) {
	mt = &MerkleTester{
		roots:     make(map[int][]byte),
		proofSets: make(map[int]map[int][][]byte),
	}
	mt.T = t

	// Fill out the data and leaves values.
	size := 16
	for i := 0; i < size; i++ {
		mt.data = append(mt.data, []byte{byte(i)})
	}
	for i := 0; i < size; i++ {
		mt.leaves = append(mt.leaves, sum(sha256.New(), append([]byte{0}, mt.data[i]...)))
	}

	// Manually build out expected Merkle root values.
	mt.roots[0] = nil
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

	// Manually build out some proof sets that should should match what the
	// Tree creates for the same values.
	mt.proofSets[1] = make(map[int][][]byte)
	mt.proofSets[1][0] = [][]byte{mt.data[0]}

	mt.proofSets[2] = make(map[int][][]byte)
	mt.proofSets[2][0] = [][]byte{
		mt.data[0],
		mt.leaves[1],
	}

	mt.proofSets[2][1] = [][]byte{
		mt.data[1],
		mt.leaves[0],
	}

	mt.proofSets[5] = make(map[int][][]byte)
	mt.proofSets[5][4] = [][]byte{
		mt.data[4],
		mt.roots[4],
	}

	mt.proofSets[6] = make(map[int][][]byte)
	mt.proofSets[6][0] = [][]byte{
		mt.data[0],
		mt.leaves[1],
		mt.join(
			mt.leaves[2],
			mt.leaves[3],
		),
		mt.join(
			mt.leaves[4],
			mt.leaves[5],
		),
	}

	mt.proofSets[6][2] = [][]byte{
		mt.data[2],
		mt.leaves[3],
		mt.roots[2],
		mt.join(
			mt.leaves[4],
			mt.leaves[5],
		),
	}

	mt.proofSets[6][4] = [][]byte{
		mt.data[4],
		mt.leaves[5],
		mt.roots[4],
	}

	mt.proofSets[6][5] = [][]byte{
		mt.data[5],
		mt.leaves[4],
		mt.roots[4],
	}

	mt.proofSets[7] = make(map[int][][]byte)
	mt.proofSets[7][5] = [][]byte{
		mt.data[5],
		mt.leaves[4],
		mt.leaves[6],
		mt.roots[4],
	}

	mt.proofSets[15] = make(map[int][][]byte)
	mt.proofSets[15][3] = [][]byte{
		mt.data[3],
		mt.leaves[2],
		mt.roots[2],
		mt.join(
			mt.join(mt.leaves[4], mt.leaves[5]),
			mt.join(mt.leaves[6], mt.leaves[7]),
		),
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
	}

	mt.proofSets[15][10] = [][]byte{
		mt.data[10],
		mt.leaves[11],
		mt.join(
			mt.leaves[8],
			mt.leaves[9],
		),
		mt.join(
			mt.join(mt.leaves[12], mt.leaves[13]),
			mt.leaves[14],
		),
		mt.roots[8],
	}

	mt.proofSets[15][13] = [][]byte{
		mt.data[13],
		mt.leaves[12],
		mt.leaves[14],
		mt.join(
			mt.join(mt.leaves[8], mt.leaves[9]),
			mt.join(mt.leaves[10], mt.leaves[11]),
		),
		mt.roots[8],
	}

	return
}

// TestBuildRoot checks that the root returned by Tree matches the manually
// created roots for all of the manually created roots.
func TestBuildRoot(t *testing.T) {
	mt := CreateMerkleTester(t)

	// Compare the results of calling Root against all of the manually
	// constructed Merkle trees.
	var tree *Tree
	for i, root := range mt.roots {
		// Fill out the tree.
		tree = New(sha256.New())
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

	// Compare the results of building a Merkle proof to all of the manually
	// constructed proofs.
	tree := New(sha256.New())
	for i, manualProveSets := range mt.proofSets {
		for j, expectedProveSet := range manualProveSets {
			// Build out the tree.
			tree = New(sha256.New())
			err := tree.SetIndex(uint64(j))
			if err != nil {
				t.Fatal(err)
			}
			for k := 0; k < i; k++ {
				tree.Push(mt.data[k])
			}

			// Get the proof and check all values.
			merkleRoot, proofSet, proofIndex, numSegments := tree.Prove()
			if bytes.Compare(merkleRoot, mt.roots[i]) != 0 {
				t.Error("incorrect Merkle root returned by Tree for indices", i, j)
			}
			if len(proofSet) != len(expectedProveSet) {
				t.Error("proof set is wrong length for indices", i, j)
				continue
			}
			if proofIndex != uint64(j) {
				t.Error("incorrect proofIndex returned for indices", i, j)
			}
			if numSegments != uint64(i) {
				t.Error("incorrect numSegments returned for indices", i, j)
			}
			for k := range proofSet {
				if bytes.Compare(proofSet[k], expectedProveSet[k]) != 0 {
					t.Error("proof set does not match expected proof set for indices", i, j, k)
				}
			}

			// Check that verification works on for the desired proof index but
			// fails for all other indices.
			if !VerifyProof(sha256.New(), merkleRoot, proofSet, proofIndex, numSegments) {
				t.Error("proof set does not verify for indices", i, j)
			}
			for k := uint64(0); k < uint64(i); k++ {
				if k == proofIndex {
					continue
				}
				if VerifyProof(sha256.New(), merkleRoot, proofSet, k, numSegments) {
					t.Error("proof set verifies for wrong index at indices", i, j, k)
				}
			}

			// Check that calling Prove a second time results in the same
			// values.
			merkleRoot2, proofSet2, proofIndex2, numSegments2 := tree.Prove()
			if bytes.Compare(merkleRoot, merkleRoot2) != 0 {
				t.Error("tree returned different merkle roots after calling Prove twice for indices", i, j)
			}
			if len(proofSet) != len(proofSet2) {
				t.Error("tree returned different proof sets after calling Prove twice for indices", i, j)
			}
			for k := range proofSet {
				if bytes.Compare(proofSet[k], proofSet2[k]) != 0 {
					t.Error("tree returned different proof sets after calling Prove twice for indices", i, j)
				}
			}
			if proofIndex != proofIndex2 {
				t.Error("tree returned different proof indexes after calling Prove twice for indices", i, j)
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
	// Get the root and proof of an empty tree.
	tree := New(sha256.New())
	root := tree.Root()
	if root != nil {
		t.Error("root of empty tree should be nil")
	}
	_, proof, _, _ := tree.Prove()
	if proof != nil {
		t.Error("proof of empty tree should be nil")
	}

	// Get the proof of a tree that hasn't reached it's index.
	err := tree.SetIndex(3)
	if err != nil {
		t.Fatal(err)
	}
	tree.Push([]byte{1})
	_, proof, _, _ = tree.Prove()
	if proof != nil {
		t.Fatal(err)
	}
	err = tree.SetIndex(2)
	if err == nil {
		t.Error("expecting error, shouldn't be able to reset a tree after pushing")
	}

	// Try nil values in VerifyProof.
	mt := CreateMerkleTester(t)
	if VerifyProof(sha256.New(), nil, mt.proofSets[1][0], 0, 1) {
		t.Error("VerifyProof should return false for nil merkle root")
	}
	if VerifyProof(sha256.New(), []byte{1}, nil, 0, 1) {
		t.Error("VerifyProof should return false for nil proof set")
	}
	if VerifyProof(sha256.New(), mt.roots[15], mt.proofSets[15][3][1:], 3, 15) {
		t.Error("VerifyProof should return false for too-short proof set")
	}
	if VerifyProof(sha256.New(), mt.roots[15], mt.proofSets[15][10][1:], 10, 15) {
		t.Error("VerifyProof should return false for too-short proof set")
	}
	if VerifyProof(sha256.New(), mt.roots[15], mt.proofSets[15][10], 15, 0) {
		t.Error("VerifyProof should return false when numLeaves is 0")
	}
}

// TestCompatibility runs BuildProof for a large set of trees, and checks that
// verify affirms each proof, while rejecting for all other indexes (this
// second half requires that all input data be unique). The test checks that
// build and verify are internally consistent, but doesn't check for actual
// correctness.
func TestCompatibility(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Brute force all trees up to size 'max'. Running time for this test is max^3.
	max := uint64(129)
	tree := New(sha256.New())
	for i := uint64(1); i < max; i++ {
		// Try with proof at every possible index.
		for j := uint64(0); j < i; j++ {
			// Push unique data into the tree.
			tree = New(sha256.New())
			err := tree.SetIndex(j)
			if err != nil {
				t.Fatal(err)
			}
			for k := uint64(0); k < i; k++ {
				tree.Push([]byte{byte(k)})
			}

			// Build the proof for the tree and run it through verify.
			merkleRoot, proofSet, proofIndex, numLeaves := tree.Prove()
			if !VerifyProof(sha256.New(), merkleRoot, proofSet, proofIndex, numLeaves) {
				t.Error("proof didn't verify for indices", i, j)
			}

			// Check that verification fails for all other indices.
			for k := uint64(0); k < i; k++ {
				if k == j {
					continue
				}
				if VerifyProof(sha256.New(), merkleRoot, proofSet, k, numLeaves) {
					t.Error("proof verified for indices", i, j, k)
				}
			}
		}
	}

	// Check that proofs on larger trees are consistent.
	for i := 0; i < 25; i++ {
		// Determine a random size for the tree up to 64M elements.
		sizeI, err := rand.Int(rand.Reader, big.NewInt(256e3))
		if err != nil {
			t.Fatal(err)
		}
		size := uint64(sizeI.Int64())

		proofIndexI, err := rand.Int(rand.Reader, sizeI)
		if err != nil {
			t.Fatal(err)
		}
		proofIndex := uint64(proofIndexI.Int64())

		// Prepare the tree.
		tree = New(sha256.New())
		err = tree.SetIndex(proofIndex)
		if err != nil {
			t.Fatal(err)
		}

		// Insert 'size' unique elements.
		for j := 0; j < int(size); j++ {
			elem := []byte(strconv.Itoa(j))
			tree.Push(elem)
		}

		// Get the proof for the tree and run it through verify.
		merkleRoot, proofSet, proofIndex, numLeaves := tree.Prove()
		if !VerifyProof(sha256.New(), merkleRoot, proofSet, proofIndex, numLeaves) {
			t.Error("proof didn't verify in long test", size, proofIndex)
		}
	}
}

// TestLeafCounts checks that the number of leaves in the tree are being
// reported correctly.
func TestLeafCounts(t *testing.T) {
	tree := New(sha256.New())
	err := tree.SetIndex(0)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, leaves := tree.Prove()
	if leaves != 0 {
		t.Error("bad reporting of leaf count")
	}

	tree = New(sha256.New())
	err = tree.SetIndex(0)
	if err != nil {
		t.Fatal(err)
	}
	tree.Push([]byte{})
	_, _, _, leaves = tree.Prove()
	if leaves != 1 {
		t.Error("bad reporting on leaf count")
	}
}

// BenchmarkSha256_4MB uses sha256 to hash 4mb of data.
func BenchmarkSha256_4MB(b *testing.B) {
	data := make([]byte, 4*1024*1024)
	_, err := rand.Read(data)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sha256.Sum256(data)
	}
}

// BenchmarkTree64_4MB creates a Merkle tree out of 4MB using a segment size of
// 64 bytes, using sha256.
func BenchmarkTree64_4MB(b *testing.B) {
	data := make([]byte, 4*1024*1024)
	_, err := rand.Read(data)
	if err != nil {
		b.Fatal(err)
	}
	segmentSize := 64

	b.ResetTimer()
	tree := New(sha256.New())
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(data)/segmentSize; j++ {
			tree.Push(data[j*segmentSize : (j+1)*segmentSize])
		}
		tree.Root()
	}
}

// BenchmarkTree4k_4MB creates a Merkle tree out of 4MB using a segment size of
// 4096 bytes, using sha256.
func BenchmarkTree4k_4MB(b *testing.B) {
	data := make([]byte, 4*1024*1024)
	_, err := rand.Read(data)
	if err != nil {
		b.Fatal(err)
	}
	segmentSize := 4096

	b.ResetTimer()
	tree := New(sha256.New())
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(data)/segmentSize; j++ {
			tree.Push(data[j*segmentSize : (j+1)*segmentSize])
		}
		tree.Root()
	}
}
