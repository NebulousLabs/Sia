package sia

import (
	"crypto/sha256"
	"errors"
	"io"
)

func HashBytes(data []byte) Hash {
	return sha256.Sum256(data)
}

// Helper function for Merkle trees; takes two hashes, concatenates them,
// and hashes the result.
func joinHash(left, right Hash) Hash {
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
//
// MerkleRoot will panic if the leaves slice is empty.
func MerkleRoot(leaves []Hash) Hash {
	if len(leaves) == 0 {
		var hash Hash
		return hash
	}

	if len(leaves) == 1 {
		return leaves[0]
	} else if len(leaves) == 2 {
		return joinHash(leaves[0], leaves[1])
	}

	// locate largest power of 2 < len(leaves)
	mid := 1
	for mid < len(leaves)/2+len(leaves)%2 {
		mid *= 2
	}

	return joinHash(MerkleRoot(leaves[:mid]), MerkleRoot(leaves[mid:]))
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
	hash = joinHash(left, right)
	return
}
