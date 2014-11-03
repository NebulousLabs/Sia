package sia

import (
	"testing"
)

func TestMerkleRoot(t *testing.T) {
	// compare MerkleRoot fn to manual hashing of a 7-leaf tree
	leaves := []Hash{Hash{0}, Hash{1}, Hash{2}, Hash{3}, Hash{4}, Hash{5}, Hash{6}}

	// calculate root manually
	manualRoot := joinHash(
		joinHash(
			joinHash(leaves[0], leaves[1]),
			joinHash(leaves[2], leaves[3]),
		),
		joinHash(
			joinHash(leaves[4], leaves[5]),
			leaves[6],
		),
	)

	if manualRoot != MerkleRoot(leaves) {
		t.Fatal("MerkleRoot hash does not match manual hash")
	}
}
