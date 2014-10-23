package main

import (
	"crypto/sha512"
	"errors"
	"io"
)

const (
	HashSize = 32
	AtomSize = 32
)

type Hash [HashSize]byte

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

// MerkleCollapse splits the provided data into segments of size AtomSize. It
// then recursively transforms these segments into a Merkle tree, and returns
// the root hash.
func MerkleCollapse(reader io.Reader, numAtoms uint16) (hash Hash, err error) {
	if numAtoms == 0 {
		err = errors.New("no data")
		return
	}
	if numAtoms == 1 {
		data := make([]byte, AtomSize)
		_, err = reader.Read(data)
		hash = HashBytes(data)
		return
	}

	// locate smallest power of 2 <= numAtoms
	var mid uint16 = 1
	for mid < numAtoms/2+numAtoms%2 {
		mid *= 2
	}

	// since we always read "left to right", no extra Seeking is necessary
	left, _ := MerkleCollapse(reader, mid)
	right, err := MerkleCollapse(reader, numAtoms-mid)
	hash = joinHash(left, right)
	return
}
