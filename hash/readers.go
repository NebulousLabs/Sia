package hash

import (
	"errors"
	"io"
)

// ReaderMerkleRoot splits the provided data into segments. It then recursively
// transforms these segments into a Merkle tree, and returns the root hash.
// See MerkleRoot for a diagram of how Merkle trees are constructed.
func ReaderMerkleRoot(reader io.Reader, numSegments uint64) (hash Hash, err error) {
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
	mid := uint64(1)
	for mid < numSegments/2+numSegments%2 {
		mid *= 2
	}

	// since we always read "left to right", no extra Seeking is necessary
	left, err := ReaderMerkleRoot(reader, mid)
	right, err := ReaderMerkleRoot(reader, numSegments-mid)
	hash = JoinHash(left, right)
	return
}

// Calculates the number of segments in the file when building a merkle tree.
// Should probably be renamed to CountLeaves() or something.
func CalculateSegments(fileSize int64) (numSegments uint64, err error) {
	if fileSize < 0 {
		err = errors.New("cannot have a negative file size")
		return
	}

	numSegments = uint64(fileSize / SegmentSize)
	if fileSize%SegmentSize != 0 {
		numSegments++
	}
	return
}

// buildProof constructs a list of hashes using the following procedure. The
// storage proof requires traversing the Merkle tree from the proofIndex node
// to the root. On each level of the tree, we must provide the hash of the
// "sister" node. (Since this is a binary tree, the sister node is the other
// node with the same parent as us.) To obtain this hash, we call
// ReaderMerkleRoot on the segment of data corresponding to the sister. This
// segment will double in size on each iteration until we reach the root.
//
// SOME SORT OF MATH PROOF FOR WHY IT WORKS WOULD BE PREFERABLE. I CAN DO THAT
// IF YOU WANT, BUT IT MIGHT INVOLVE ME COMPLETELY REWRITING IT ALL SINCE THE
// PROOF MIGHT LOOK DIFFERENT THAN WHAT YOUVE IMPLEMENTED, EVEN IF YOUR
// IMPLEMENTATION IS CORRECT.
func BuildReaderProof(rs io.ReadSeeker, numSegments, proofIndex uint64) (baseSegment [SegmentSize]byte, hashSet []Hash, err error) {
	// Find the base segment that is being requested.
	if _, err = rs.Seek(int64(proofIndex)*int64(SegmentSize), 0); err != nil {
		return
	}
	if _, err = rs.Read(baseSegment[:]); err != nil {
		return
	}

	// THIS FOR LOOP IS MESSY AND I DONT UNDERSTAND IT VERY WELL. THERE SHOULD
	// BE MORE COMMENTS AND SOME EXPLAINATION FOR WHY ITS CORRECT.
	// Construct the hash set that proves the base segment is a part of the
	// merkle tree of the reader. (Verifier needs to know the merkle root of
	// the file in advance.)
	for size := uint64(1); size < numSegments; size <<= 1 {
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
		var h Hash
		h, err = ReaderMerkleRoot(rs, truncSize)
		if err != nil {
			return
		}
		hashSet = append(hashSet, h)
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
//
// SOME SORT OF MATH PROOF FOR WHY IT WORKS WOULD BE PREFERABLE. I CAN DO THAT
// IF YOU WANT, BUT IT MIGHT INVOLVE ME COMPLETELY REWRITING IT ALL SINCE THE
// PROOF MIGHT LOOK DIFFERENT THAN WHAT YOUVE IMPLEMENTED, EVEN IF YOUR
// IMPLEMENTATION IS CORRECT.
func VerifyReaderProof(baseSegment [SegmentSize]byte, hashSet []Hash, numSegments, proofIndex uint64, expectedRoot Hash) bool {
	h := HashBytes(baseSegment[:])

	// NEED MORE COMMENTS, AND SOME EXPLAINATION FOR WHY ITS CORRECT.
	depth := uint64(0)
	for (1 << depth) < numSegments {
		depth++
	}
	// does this hashset contain orphans?
	orphanFlag := len(hashSet) < int(depth)

	for i := uint64(0); i < depth; i++ {
		// is this an orphan?
		// (not sure why this works...)
		if orphanFlag && (numSegments-1)&(1<<i) == 0 {
			continue
		}
		// left or right?
		if proofIndex&(1<<i) == 0 {
			h = JoinHash(h, hashSet[0])
		} else {
			h = JoinHash(hashSet[0], h)
		}
		hashSet = hashSet[1:]
	}

	return h == expectedRoot
}
