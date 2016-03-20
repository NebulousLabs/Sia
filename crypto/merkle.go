package crypto

// TODO: It's likely that all of the readers in this file can be replaced with
// simple byte slices, I believe that in the vast majority of cases things are
// now handled one sector at a time, meaning that the callers are actually
// creating new readers out of byte slices the majority of the time in order to
// be able to interact with this file.

import (
	"io"

	"github.com/NebulousLabs/Sia/encoding"

	"github.com/NebulousLabs/merkletree"
)

const (
	SegmentSize = 64 // number of bytes that are hashed to form each base leaf of the Merkle tree
)

// MerkleTree wraps the merkletree.Tree type, providing convenient
// Sia-specific functionality.
type MerkleTree struct {
	merkletree.Tree
}

// NewTree returns a tree object that can be used to get the Merkle root of a
// dataset.
func NewTree() *MerkleTree {
	return &MerkleTree{*merkletree.New(NewHash())}
}

// PushObject encodes and adds the hash of the encoded object to the tree as a
// leaf.
func (t *MerkleTree) PushObject(obj interface{}) {
	t.Push(encoding.Marshal(obj))
}

// Root returns the Merkle root of all the objects pushed to the tree.
func (t *MerkleTree) Root() (h Hash) {
	copy(h[:], t.Tree.Root())
	return
}

// CachedMerkleTree wraps the merkletree.CachedTree type, providing convenient
// Sia-specific functionality.
type CachedMerkleTree struct {
	merkletree.CachedTree
}

// NewCached returns a new cached tree object. A tree of height 1 will have 2
// elements in each subtree, at height 2 there are 4 elements, etc.
func NewCachedTree(height uint64) *CachedMerkleTree {
	return &CachedMerkleTree{*merkletree.NewCachedTree(NewHash(), height)}
}

// Prove returns a proof that the data at the previously established index is a
// part of the tree. The input is a proof that the data is in the sub-tree.
func (ct *CachedMerkleTree) Prove(base []byte, cachedHashSet []Hash) []Hash {
	// Turn the input in to a proof set that will be recognized by the high
	// level tree.
	cachedProofSet := make([][]byte, len(cachedHashSet)+1)
	cachedProofSet[0] = base
	for i := range cachedHashSet {
		cachedProofSet[i+1] = cachedHashSet[i][:]
	}
	_, proofSet, _, _ := ct.CachedTree.Prove(cachedProofSet)

	// convert proofSet to base and hashSet
	hashSet := make([]Hash, len(proofSet)-1)
	for i, proof := range proofSet[1:] {
		copy(hashSet[i][:], proof)
	}
	return hashSet
}

// Push pushes a subtree Merkle root into a cached Merkle tree.
func (ct *CachedMerkleTree) Push(h Hash) {
	ct.CachedTree.Push(h[:])
}

// Root returns the Merkle root of all the objects pushed to the tree.
func (ct *CachedMerkleTree) Root() (h Hash) {
	copy(h[:], ct.CachedTree.Root())
	return
}

// Calculates the number of leaves in the file when building a Merkle tree.
func CalculateLeaves(fileSize uint64) uint64 {
	numSegments := fileSize / SegmentSize
	if fileSize == 0 || fileSize%SegmentSize != 0 {
		numSegments++
	}
	return numSegments
}

// ReaderMerkleRoot returns the Merkle root of a reader.
func ReaderMerkleRoot(r io.Reader) (h Hash, err error) {
	root, err := merkletree.ReaderRoot(r, NewHash(), SegmentSize)
	if err != nil {
		return
	}
	copy(h[:], root)
	return
}

// BuildReaderProof will build a storage proof when given a reader.
func BuildReaderProof(r io.Reader, proofIndex uint64) (base []byte, hashSet []Hash, err error) {
	_, proofSet, _, err := merkletree.BuildReaderProof(r, NewHash(), SegmentSize, proofIndex)
	if err != nil {
		return
	}
	// convert proofSet to base and hashSet
	base = proofSet[0]
	hashSet = make([]Hash, len(proofSet)-1)
	for i, proof := range proofSet[1:] {
		copy(hashSet[i][:], proof)
	}
	return
}

// VerifySegment will verify that a segment, given the proof, is a part of a
// Merkle root.
func VerifySegment(base []byte, hashSet []Hash, numSegments, proofIndex uint64, root Hash) bool {
	// convert base and hashSet to proofSet
	proofSet := make([][]byte, len(hashSet)+1)
	proofSet[0] = base
	for i := range hashSet {
		proofSet[i+1] = hashSet[i][:]
	}
	return merkletree.VerifyProof(NewHash(), root[:], proofSet, proofIndex, numSegments)
}
