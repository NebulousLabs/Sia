package crypto

import (
	"errors"
	"io"

	"github.com/NebulousLabs/Sia/encoding"

	"github.com/NebulousLabs/merkletree"
)

const (
	SegmentSize = 64 // number of bytes that are hashed to form each base leaf of the Merkle tree
)

type tree struct {
	*merkletree.Tree
}

// NewTree returns a tree object that can be used to get the merkle root of a
// dataset.
func NewTree() tree {
	return tree{merkletree.New(NewHash())}
}

// PushObject encodes and adds the hash of the encoded object to the tree as a
// leaf.
func (t tree) PushObject(obj interface{}) {
	t.Push(encoding.Marshal(obj))
}

// Root returns the Merkle root of all the objects pushed to the tree.
func (t tree) Root() (h Hash) {
	copy(h[:], t.Tree.Root())
	return
}

// MerkleRoot calculates the "root hash" formed by repeatedly concatenating
// and hashing a binary tree of hashes. If the number of leaves is not a
// power of 2, the orphan hash(es) are not rehashed. Examples:
//
//       ┌───┴──┐       ┌────┴───┐         ┌─────┴─────┐
//    ┌──┴──┐   │    ┌──┴──┐     │      ┌──┴──┐     ┌──┴──┐
//  ┌─┴─┐ ┌─┴─┐ │  ┌─┴─┐ ┌─┴─┐ ┌─┴─┐  ┌─┴─┐ ┌─┴─┐ ┌─┴─┐   │
//     (5-leaf)         (6-leaf)             (7-leaf)
func MerkleRoot(leaves [][]byte) (h Hash) {
	tree := merkletree.New(NewHash())
	for _, leaf := range leaves {
		tree.Push(leaf)
	}
	copy(h[:], tree.Root())
	return
}

// Calculates the number of leaves in the file when building a Merkle tree.
func CalculateLeaves(fileSize uint64) (numSegments uint64) {
	numSegments = fileSize / SegmentSize
	if fileSize%SegmentSize != 0 {
		numSegments++
	}
	return
}

// ReaderMerkleRoot returns the merkle root of a reader.
func ReaderMerkleRoot(r io.Reader) (h Hash, err error) {
	root, err := merkletree.ReaderRoot(r, NewHash(), SegmentSize)
	if err != nil {
		return
	}
	copy(h[:], root)
	return
}

// BuildReaderProof will build a storage proof when given a reader.
func BuildReaderProof(r io.Reader, proofIndex uint64) (base [SegmentSize]byte, hashSet []Hash, err error) {
	_, proofSet, _, err := merkletree.BuildReaderProof(r, NewHash(), SegmentSize, proofIndex)
	if err != nil {
		return
	}
	if len(proofSet) == 0 {
		return base, nil, errors.New("reader was empty")
	}
	// convert proofSet to base and hashSet
	copy(base[:], proofSet[0])
	hashSet = make([]Hash, len(proofSet)-1)
	for i, proof := range proofSet[1:] {
		copy(hashSet[i][:], proof)
	}
	return
}

// VerifySegment will verify that a segment, given the proof, is a part of a
// merkle root.
func VerifySegment(base [SegmentSize]byte, hashSet []Hash, numSegments, proofIndex uint64, root Hash) bool {
	// convert base and hashSet to proofSet
	proofSet := make([][]byte, len(hashSet)+1)
	proofSet[0] = base[:]
	for i := range hashSet {
		proofSet[i+1] = hashSet[i][:]
	}
	return merkletree.VerifyProof(NewHash(), root[:], proofSet, proofIndex, numSegments)
}
