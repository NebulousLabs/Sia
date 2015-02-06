package crypto

const (
	SegmentSize = 64 // number of bytes that are hashed to form each base leaf of the Merkle tree
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
