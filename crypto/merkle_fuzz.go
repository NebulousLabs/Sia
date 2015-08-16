// +build gofuzz

package crypto

import (
	"bytes"
)

// FuzzBuildReaderProof is called by go-fuzz to look for ways to construct a
// merkle proof that is invalid.
func FuzzBuildReaderProof(data []byte) int {
	if len(data) < 2 {
		return -1
	}
	proofIndex := uint64(data[0]) + uint64(data[1])*256
	data = data[2:]
	dataLen := uint64(len(data))

	base, hashSet, err := BuildReaderProof(bytes.NewReader(data), proofIndex)
	if err == nil && dataLen <= proofIndex*64 {
		panic("an error should be returned on the data")
	} else if err != nil && dataLen > proofIndex*64 {
		panic(err)
	} else if err != nil {
		return 0
	}
	root, err := ReaderMerkleRoot(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	leaves := CalculateLeaves(dataLen)
	if !VerifySegment(base, hashSet, leaves, proofIndex, root) {
		panic("data didn't verify")
	}
	return 1
}
