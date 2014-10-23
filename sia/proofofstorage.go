package sia

import (
	"io"
)

// A StorageProof contains the data and hashes needed to reconstruct a Merkle root.
type StorageProof struct {
	AtomBase  [AtomSize]byte
	HashStack []*Hash
}

// buildProof constructs a list of hashes using the following procedure. The
// storage proof requires traversing the Merkle tree from the proofIndex node
// to the root. On each level of the tree, we must provide the hash of "sister"
// node. (Since this is a binary tree, the sister node is the other node with
// the same parent as us.) To obtain this hash, we call MerkleCollapse on the
// segment of data corresponding to the sister. This segment will double in
// size on each iteration until we reach the root.
func buildProof(rs io.ReadSeeker, numAtoms, proofIndex uint16) (sp StorageProof, err error) {
	// get AtomBase
	if _, err = rs.Seek(int64(proofIndex)*int64(AtomSize), 0); err != nil {
		return
	}
	if _, err = rs.Read(sp.AtomBase[:]); err != nil {
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
	for size := uint16(1); size < numAtoms; size <<= 1 {
		// determine index
		i := sisterIndex(size)
		if i >= numAtoms {
			// append dummy hash
			sp.HashStack = append(sp.HashStack, nil)
			continue
		}

		// seek to beginning of segment
		rs.Seek(int64(i)*int64(AtomSize), 0)

		// truncate number of atoms to read, if necessary
		truncSize := size
		if i+size > numAtoms {
			truncSize = numAtoms - i
		}

		// calculate and append hash
		var h Hash
		h, err = MerkleCollapse(rs, truncSize)
		if err != nil {
			return
		}
		sp.HashStack = append(sp.HashStack, &h)
	}

	return
}

// verifyProof traverses a StorageProof, hashing elements together to produce
// the root-level hash, which is then checked against the expected result.
// Care must be taken to ensure that the correct ordering is used when
// concatenating hashes.
func verifyProof(sp StorageProof, proofIndex uint16, expected Hash) bool {
	h := HashBytes(sp.AtomBase[:])

	var size uint16 = 1
	for i := 0; i < len(sp.HashStack); i, size = i+1, size*2 {
		// skip dummy hashes
		if sp.HashStack[i] == nil {
			continue
		}
		if proofIndex%(size*2) < size { // base is on the left branch
			h = joinHash(h, *sp.HashStack[i])
		} else {
			h = joinHash(*sp.HashStack[i], h)
		}
	}

	return h == expected
}
