package sia

import (
	"io"

	"github.com/NebulousLabs/Andromeda/hash"
)

// Calculates the number of segments in the file when building a merkle tree.
// Should probably be renamed to CountLeaves() or something.
func calculateSegments(fileSize int64) (numSegments uint16) {
	numSegments = uint16(fileSize / SegmentSize)
	if fileSize%SegmentSize != 0 {
		numSegments++
	}
	return
}

// buildProof constructs a list of hashes using the following procedure. The
// storage proof requires traversing the Merkle tree from the proofIndex node
// to the root. On each level of the tree, we must provide the hash of the "sister"
// node. (Since this is a binary tree, the sister node is the other node with
// the same parent as us.) To obtain this hash, we call MerkleFile on the
// segment of data corresponding to the sister. This segment will double in
// size on each iteration until we reach the root.
func buildProof(rs io.ReadSeeker, numSegments, proofIndex uint16) (sp StorageProof, err error) {
	// get base segment
	if _, err = rs.Seek(int64(proofIndex)*int64(SegmentSize), 0); err != nil {
		return
	}
	if _, err = rs.Read(sp.Segment[:]); err != nil {
		return
	}

	// calculate hashes of each sister
	for size := uint16(1); size < numSegments; size <<= 1 {
		// determine sister index
		// I'd love to simplify this somehow...
		i := size - proofIndex&size + proofIndex/(size<<1)*(size<<1)

		// for "orphan" leaves, the hash is omitted. This omission can
		// be detected and accounted for during verification, provided
		// the verifier knows the value of numSegments.
		if i >= numSegments {
			continue
		}

		// seek to beginning of segment
		rs.Seek(int64(i)*int64(SegmentSize), 0)

		// truncate number of atoms to read, if necessary
		truncSize := size
		if i+size > numSegments {
			truncSize = numSegments - i
		}

		// calculate and append hash
		var h hash.Hash
		h, err = hash.MerkleFile(rs, truncSize)
		if err != nil {
			return
		}
		sp.HashSet = append(sp.HashSet, h)
	}

	return
}

// verifyProof traverses a StorageProof, hashing elements together to produce
// the root-level hash, which is then checked against the expected result.
// Care must be taken to ensure that the correct ordering is used when
// concatenating hashes.
//
// Implementation note: the "left-right" ordering for a given proofIndex can
// be determined from its little-endian binary representation, where a 0
// indicates "left" and a 1 indicates "right." However, this must be modified
// slightly for trees with "orphans," since they cause certain lefts/rights to
// be skipped. As it turns out, the branches to skip can be determined from the
// binary representation of numSegments-1, where a 0 indicates "skip" and a 1
// indicates "keep." I don't know why this works, I just noticed the pattern.
func verifyProof(sp StorageProof, numSegments, proofIndex uint16, expected hash.Hash) bool {
	h := hash.HashBytes(sp.Segment[:])

	var depth uint16 = 0
	for (1 << depth) < numSegments {
		depth++
	}
	// does this hashset contain orphans?
	orphanFlag := len(sp.HashSet) < int(depth)

	for i := uint16(0); i < depth; i++ {
		// is this an orphan?
		// (not sure why this works...)
		if orphanFlag && (numSegments-1)&(1<<i) == 0 {
			continue
		}
		// left or right?
		if proofIndex&(1<<i) == 0 {
			h = hash.JoinHash(h, sp.HashSet[0])
		} else {
			h = hash.JoinHash(sp.HashSet[0], h)
		}
		sp.HashSet = sp.HashSet[1:]
	}

	return h == expected
}
