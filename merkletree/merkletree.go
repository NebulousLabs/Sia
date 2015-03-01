// merkletree provides tools for calculating the Merkle root of a dataset, for
// creating a proof that a piece of data is in a Merkle tree of a given root,
// and for verifying proofs that a piece of data is in a Merkle tree of a given
// root.
package merkletree

import (
	"bytes"
	"hash"
)

// A Tree takes data as leaves and returns the merkle root. Each call to 'Push'
// adds one leaf to the merkle tree. Calling 'Root' returns the Merkle root.
// The Tree also constructs proof that a single leaf is a part of the tree. The
// leaf can be chosen with 'SetIndex'.
type Tree struct {
	head *subTree
	hash hash.Hash

	// Variables to help build proofs that the data at 'proveIndex' is in the
	// merkle tree.
	currentIndex int
	proveIndex   int
	proveSet     [][]byte
}

// A subTree is a subTree in the Tree. 'height' refers to how tall the subTree
// is. The children of the tree are not accessible, as they have already been
// hashed into 'sum'. 'next' is the next subTree, and is guaranteed to have
// a higher height unless it is nil.

// A subTree contains the merkle root of a complete (2^n leaves) subTree of
// the Tree. 'sum' is the Merkle root of the subTree. If 'next' is not nil, it
// will be a tree with a higher height.
type subTree struct {
	next   *subTree
	height int
	sum    []byte
}

// sum returns the hash of the input data.
func sum(h hash.Hash, data []byte) []byte {
	if data == nil {
		return nil
	}

	h.Write(data)
	result := h.Sum(nil)
	h.Reset()
	return result
}

// join takes two byte slices, appends them, hashes them, and then returns the
// result.
func join(h hash.Hash, a, b []byte) []byte {
	return sum(h, append(a, b...))
}

// New initializes a Tree with a hash object, which will be used when hashing
// the input.
func New(h hash.Hash) *Tree {
	return &Tree{
		hash: h,
	}
}

// Reset returns the tree to its inital, empty state.
func (t *Tree) Reset() {
	t.head = nil
	t.currentIndex = 0
	t.proveIndex = 0
	t.proveSet = nil
}

// SetIndex resets the tree, and then sets the index for which a proof that the
// element is in the Tree will be built.
func (t *Tree) SetIndex(i int) {
	t.Reset()
	t.proveIndex = i
}

// Push adds a leaf to the tree by hashing the input and then inserting the
// result as a leaf.
func (t *Tree) Push(data []byte) {
	// The first element of a proof is the original data at a leaf. If the
	// current index is the index for which we are creating a proof, save the
	// data.
	if t.currentIndex == t.proveIndex {
		t.proveSet = append(t.proveSet, data)
	}

	// Hash the data, creating a subTree of height 1.
	current := &subTree{
		height: 1,
		sum:    sum(t.hash, data),
	}
	// Check the height of the next subTree. If the height of the next subTree
	// is the same as the height of the current subTree, combine the two
	// subTrees to create a subTree with a height that is 1 greater.
	for t.head != nil && current.height == t.head.height {
		// When creating a proof for a specific index, you need to collect one
		// hash at each height of the tree, and that hash will be found in the
		// same subTree as the initial leaf. Before we hit that index, this
		// logic will be ignored because len(proveSet) will be 0. After we hit
		// that index, len(proveSet) will be one. From that point forward,
		// every time there are two subTrees (the current one and the previous
		// one) that have a height equal to len(proveSet) we will need to grab
		// one of the roots and add it to the prove set.
		if current.height == len(t.proveSet) {
			// Either the root of the current subTree or the root of the
			// previous subTree needs to be added to the proof set. We want to
			// grab the root of the subTree that does not contain
			// 't.proveIndex'. We do this by finding the starting index of the
			// current subTree and comparing it to 't.proveInex'.
			//
			// The start of the first subTree can be determined by rounding
			// the currentIndex down to the nearest (2^height). This represents
			// the combined size of the two trees, as a tree of height 1 was
			// built from only 1 leaf.
			combinedSize := int(1 << uint(current.height))
			previousStart := (t.currentIndex / combinedSize) * combinedSize
			currentStart := previousStart + (combinedSize / 2)
			if t.proveIndex < currentStart {
				t.proveSet = append(t.proveSet, current.sum)
			} else {
				t.proveSet = append(t.proveSet, t.head.sum)
			}
		}

		// Join the two subTrees into one subTree with a greater height. Then
		// compare the new subTree to the next subTree.
		current.sum = join(t.hash, t.head.sum, current.sum)
		current.height++
		t.head = t.head.next
	}

	// Add the subTree to the top of the stack.
	current.next = t.head
	t.head = current
	t.currentIndex++
}

// Root returns the Merkle root of the data that has been pushed into the Tree.
// Asking for the root when no data has been added will return nil. The tree is
// left unaltered.
func (t *Tree) Root() (root []byte) {
	// If the Tree is empty, return nil.
	if t.head == nil {
		return nil
	}

	// The root is formed by hashing together subTrees in order from least in
	// height to greatest in height. To preserve ordering, the larger subTree
	// needs to be first in the combination.
	current := t.head
	root = current.sum
	for current.next != nil {
		root = join(t.hash, current.next.sum, root)
		current = current.next
	}
	return root
}

// Prove returns a proof that the data at index 'proveIndex' is an element in
// the current Tree. The proof will be invalid if any more elements are added
// to the tree after calling Prove. The tree is left unaltered.
func (t *Tree) Prove() (h hash.Hash, merkleRoot []byte, proveSet [][]byte, proveIndex int, numLeaves int) {
	// Return nil if the Tree is empty, or if the proveIndex hasn't yet been
	// reached.
	if t.head == nil || len(t.proveSet) == 0 {
		return h, t.Root(), nil, t.proveIndex, t.currentIndex
	}
	proveSet = t.proveSet

	// The hashes have already been provided for the largest complete subTree
	// that contains 't.ProveIndex'. If 't.CurrentIndex' is a power of two, we
	// are already finshed. Otherwise, two sets of hashes remain which need to
	// be added to the proof. The first is the hashes of the smaller subTrees.
	// All of the smaller subTrees need to be combined, and then that hash
	// needs to be saved. The second is the larger subTrees. The root of each
	// of the larger subTrees needs to be saved. The subTree with the prove
	// index will have a height equal to the current length of the prove set.

	// Iterate through all of the smaller subTrees and combine them.
	current := t.head
	sum := current.sum
	for current.next != nil && current.next.height < len(proveSet) {
		// Combine this subTree with the next subTree.
		sum = join(t.hash, current.next.sum, sum)
		current = current.next
	}

	// If the current subTree is the last subTree before the subTree containing
	// the prove index, add the root of the subTree to the prove set.
	if current.next != nil && current.next.height == len(proveSet) {
		proveSet = append(proveSet, sum)
		current = current.next
	}

	// The subTree containing the prove index needs to be skipped.
	current = current.next

	// Now add the roots of all subTrees that are larger than the subTree
	// containing the proof index.
	for current != nil {
		proveSet = append(proveSet, current.sum)
		current = current.next
	}
	return t.hash, t.Root(), proveSet, t.proveIndex, t.currentIndex
}

// VerifyProof takes a merkle, a proveSet, and a proveIndex and returns true if
// the first element of the prove set is a leaf of data in the merkle root.
func VerifyProof(h hash.Hash, merkleRoot []byte, proveSet [][]byte, proveIndex int, numLeaves int) bool {
	if numLeaves == 0 {
		return true
	}
	if len(proveSet) == 0 || merkleRoot == nil {
		return false
	}

	// Determine the size of the largest full subTree (a tree with 2^n leaves)
	// that contains the proveIndex.
	largerSubTrees := 0
	sum := sum(h, proveSet[0])
	proveSet = proveSet[1:]
	for {
		// Determine the size of the largest remaining subTree.
		subTreeSize := 1
		for subTreeSize*2 <= numLeaves {
			subTreeSize *= 2
		}
		if proveIndex < subTreeSize {
			// We have found the subTree that contains the prove index. Build
			// up the proof inside of the complete subTree, where we don't need
			// to worry about edge cases.
			height := 1
			for int(1<<uint(height)) <= subTreeSize {
				heightSize := int(1 << uint(height))
				heightStart := (proveIndex / heightSize) * heightSize
				mid := heightStart + (heightSize / 2)
				if len(proveSet) == 0 {
					return false
				}
				if proveIndex < mid {
					sum = join(h, sum, proveSet[0])
				} else {
					sum = join(h, proveSet[0], sum)
				}
				height++
				proveSet = proveSet[1:]
			}

			// Check if there's a smaller subTree.
			if subTreeSize < numLeaves {
				if len(proveSet) == 0 {
					return false
				}
				sum = join(h, sum, proveSet[0])
				proveSet = proveSet[1:]
			}
			break
		}
		largerSubTrees++
		proveIndex -= subTreeSize
		numLeaves -= subTreeSize
	}

	// Add for each larger subTree.
	for i := 0; i < largerSubTrees; i++ {
		if len(proveSet) == 0 {
			return false
		}
		sum = join(h, proveSet[0], sum)
		proveSet = proveSet[1:]
	}

	// If there are still elements remaining in the prove set, return false.
	if len(proveSet) != 0 {
		return false
	}

	if bytes.Compare(sum, merkleRoot) == 0 {
		return true
	} else {
		return false
	}
}
