package sia

import (
	"io"
)

// buildProof constructs a list of hashes using the following procedure. The
// storage proof requires traversing the Merkle tree from the proofIndex node
// to the root. On each level of the tree, we must provide the hash of "sister"
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

	// sisterIndex helper function:
	//   if the sector is divided into segments of length 'size' and
	//   grouped pairwise, then proofIndex lies inside a segment
	//   that is one half of a pair. sisterIndex returns the index
	//   where the other half begins.
	//   e.g.: (5, 1) -> 4, (5, 2) -> 6, (5, 4) -> 0, ...
	sisterIndex := func(size uint16) uint16 {
		if proofIndex%(size*2) < size { // left child or right child?
			return (proofIndex/size + 1) * size
		} else {
			return (proofIndex/size - 1) * size
		}
	}

	// calculate hashes of each sister
	for size := uint16(1); size < numSegments; size <<= 1 {
		// determine index
		i := sisterIndex(size)

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
		var h Hash
		h, err = MerkleFile(rs, truncSize)
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
// slightly for "orphan" leaves by skipping the first n "missing" hashes, where
// n is the depth of the Merkle tree minus the length of the proof's hash set.
func verifyProof(sp StorageProof, numSegments, proofIndex uint16, expected Hash) bool {
	h := HashBytes(sp.Segment[:])

	var depth uint16 = 0
	for (1 << depth) < numSegments {
		depth++
	}

	for i := depth - uint16(len(sp.HashSet)); i < depth; i++ {
		if proofIndex&(1<<i) == 0 { // left
			h = joinHash(h, sp.HashSet[0])
		} else {
			h = joinHash(sp.HashSet[0], h)
		}
		sp.HashSet = sp.HashSet[1:]
	}

	return h == expected
}
