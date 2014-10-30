package sia

import (
	"crypto/sha512"
	"errors"
	"io"
)

func HashBytes(data []byte) (h Hash) {
	hash512 := sha512.Sum512(data)
	copy(h[:], hash512[:])
	return
}

// Helper function for merkle trees; takes two hashes, appends them, and then
// hashes their sum.
func joinHash(left, right Hash) Hash {
	return HashBytes(append(left[:], right[:]...))
}

// Takes as input a slice of hashes, which are the leaves of the merkle tree.
// Produces the root hash from these leaves. The hashes correspond to some data
// structure underneath. This function is useful for building merkle trees out
// of arbitrary stuff, like transactions.
func BuildMerkleTree(leaves []Hash) (hash Hash, err error) {
	if len(leaves) == 0 {
		err = errors.New("Cannot build tree of of length 1 object")
		return
	}

	hash = leaves[0]
	return
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

func (t *Transaction) Hash() Hash {
	// version, hash of arb data, miner fee, each input, each output, each file contract, each sp, each sig
	// allows you to selectively reveal pieces of a transaction? But what good is that?

	return HashBytes(MarshalAll(
		HashBytes(t.ArbitraryData),
		uint64(t.MinerFee),
		// Inputs
		// Outputs
		// File Contracts
		// Storage Proofs
		// Signatures
	))
}
