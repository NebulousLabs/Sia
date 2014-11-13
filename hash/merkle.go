package hash

import (
	"errors"
	"io"
)

// Helper function for Merkle trees; takes two hashes, concatenates them,
// and hashes the result.
func JoinHash(left, right Hash) Hash {
	return HashBytes(append(left[:], right[:]...))
}

// MerkleRoot calculates the "root hash" formed by repeatedly concatenating
// and hashing a binary tree of hashes. If the number of leaves is not a
// power of 2, the orphan hash(es) are not rehashed. Examples:
//
//       ┌───┴──┐       ┌────┴───┐         ┌─────┴─────┐
//    ┌──┴──┐   │    ┌──┴──┐     │      ┌──┴──┐     ┌──┴──┐
//  ┌─┴─┐ ┌─┴─┐ │  ┌─┴─┐ ┌─┴─┐ ┌─┴─┐  ┌─┴─┐ ┌─┴─┐ ┌─┴─┐   │
//     (5-leaf)         (6-leaf)             (7-leaf)
func MerkleRoot(leaves []Hash) Hash {
	switch len(leaves) {
	case 0:
		return Hash{}
	case 1:
		return leaves[0]
	case 2:
		return JoinHash(leaves[0], leaves[1])
	}

	// locate largest power of 2 < len(leaves)
	mid := 1
	for mid < len(leaves)/2+len(leaves)%2 {
		mid *= 2
	}

	return JoinHash(MerkleRoot(leaves[:mid]), MerkleRoot(leaves[mid:]))
}

// MerkleFile splits the provided data into segments. It then recursively
// transforms these segments into a Merkle tree, and returns the root hash.
// See MerkleRoot for a diagram of how Merkle trees are constructed.
func MerkleFile(reader io.Reader, numSegments uint16) (hash Hash, err error) {
	if numSegments == 0 {
		err = errors.New("no data")
		return
	}
	if numSegments == 1 {
		data := make([]byte, SegmentSize)
		n, _ := reader.Read(data)
		if n == 0 {
			err = errors.New("no data")
		} else {
			hash = HashBytes(data)
		}
		return
	}

	// locate smallest power of 2 < numSegments
	var mid uint16 = 1
	for mid < numSegments/2+numSegments%2 {
		mid *= 2
	}

	// since we always read "left to right", no extra Seeking is necessary
	left, _ := MerkleFile(reader, mid)
	right, err := MerkleFile(reader, numSegments-mid)
	hash = JoinHash(left, right)
	return
}
