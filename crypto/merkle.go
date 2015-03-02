package crypto

// TODO: Give this file a lot more love. And maybe break it into its own
// package.

import (
	"bytes"
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

func (t tree) PushObject(obj interface{}) {
	t.Push(encoding.Marshal(obj))
}

func (t tree) Root() (h Hash) {
	copy(h[:], t.Tree.Root())
	return
}

func NewTree() tree {
	return tree{merkletree.New(NewHash())}
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

// Calculates the number of segments in the file when building a Merkle tree.
// Should probably be renamed to CountLeaves() or something.
func CalculateSegments(fileSize uint64) (numSegments uint64) {
	numSegments = fileSize / SegmentSize
	if fileSize%SegmentSize != 0 {
		numSegments++
	}
	return
}

func BytesMerkleRoot(data []byte) (Hash, error) {
	return ReaderMerkleRoot(bytes.NewReader(data))
}

func ReaderMerkleRoot(r io.Reader) (h Hash, err error) {
	root, err := merkletree.ReaderRoot(r, NewHash(), SegmentSize)
	if err != nil {
		return
	}
	copy(h[:], root)
	return
}

func BuildReaderProof(r io.Reader, proofIndex uint64) (base [SegmentSize]byte, hashSet []Hash, err error) {
	_, proofSet, _, err := merkletree.BuildReaderProof(r, NewHash(), SegmentSize, proofIndex)
	if err != nil {
		return
	}
	// convert proofSet to base and hashSet
	copy(base[:], proofSet[0])
	hashSet = make([]Hash, len(proofSet)-1)
	for i, proof := range proofSet[1:] {
		copy(hashSet[i][:], proof)
	}
	return
}

func VerifySegment(base [SegmentSize]byte, hashSet []Hash, numSegments, proofIndex uint64, root Hash) bool {
	// convert base and hashSet to proofSet
	proofSet := make([][]byte, len(hashSet)+1)
	proofSet[0] = base[:]
	for i := range hashSet {
		proofSet[i+1] = hashSet[i][:]
	}
	return merkletree.VerifyProof(NewHash(), root[:], proofSet, proofIndex, numSegments)
}
