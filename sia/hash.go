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
// and hashing a binary tree of hashes.  If the number of leaves is not a
// power of 2, the orphan hash(es) are concatenated with a single 0 byte.
// MerkleRoot will panic if the leaves slice is empty.
func MerkleRoot(leaves []Hash) Hash {
	if len(leaves) == 1 {
		return HashBytes(append(leaves[0][:], 0))
	} else if len(leaves) == 2 {
		return joinHash(leaves[0], leaves[1])
	}

	// locate smallest power of 2 < len(leaves)
	mid := 1
	for mid < len(leaves)/2+len(leaves)%2 {
		mid *= 2
	}

	return joinHash(MerkleRoot(leaves[:mid]), MerkleRoot(leaves[mid:]))
}

// MerkleFile splits the provided data into segments. It then recursively
// transforms these segments into a Merkle tree, and returns the root hash.
func MerkleFile(reader io.Reader, numAtoms uint16) (hash Hash, err error) {
	if numAtoms == 0 {
		err = errors.New("no data")
		return
	}
	if numAtoms == 1 {
		data := make([]byte, SegmentSize)
		n, _ := reader.Read(data)
		if n == 0 {
			err = errors.New("no data")
		} else {
			hash = HashBytes(data)
		}
		return
	}

	// locate smallest power of 2 < numAtoms
	var mid uint16 = 1
	for mid < numAtoms/2+numAtoms%2 {
		mid *= 2
	}

	// since we always read "left to right", no extra Seeking is necessary
	left, _ := MerkleFile(reader, mid)
	right, err := MerkleFile(reader, numAtoms-mid)
	hash = joinHash(left, right)
	return
}
