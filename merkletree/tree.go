package merkletree

import (
	"bytes"
	"hash"
)

// Tree takes data one input at a time, hashes the data and then adds the data
// as a leaf to the merkle tree. The merkle tree is assembled as data is
// inserted, which means the original data cannot be recovered from the tree.
//
// Tree works like a stack. When data is added, it is hashed into a smaller
// merkle tree of height 1 (the hash of a leaf) and then added to the stack.
// Before it is added, the height of the next sub-tree is checked. If the
// height is the same, then the two subtrees have their values appended and
// hashed, creating a new subtree that has a height 1 greater than the prior
// sub trees. Before it gets pushed, the next subtree is checked, and so on.
// This guarantees that there will only ever be log(n) subtrees (of constant
// size), where n is the number of times 'Push' has been called.
//
// While being built, a tree can provide a proof that exactly one element is in
// the set of data. Which element must be decided in advance.  The proof is
// built while the tree is being constructed.  The current index of the data
// being added is saved to the tree, and nothing else needs to be added to the
// proof until we hit the index that we are creating the proof for. When we get
// to that index, we save the data before hashing it. At that point, our
// proofSet has a length of 1. From that point forward, the only time that a
// hash needs to be added to the proof set is when the height of the new
// subTree and the next subTree are both equal to the length of the proofSet.
// You only need 1 hash at each height in the Tree, and it will always appear
// at this moment. From there, you have 2 hashes that you can potentially take.
// You want the hash that isn't in the same subTree as index you are creating
// the proof for.

// A Tree builds a Merkle tree. Add data one slice at a time, and the tree will
// hash the data and build out the Merkle tree using log(n) memory, where n is
// the number of times 'Push' is called.
type Tree struct {
	head *subTree
	hash hash.Hash

	// Variables to help build proofs that the data at 'proveIndex' is in the
	// merkle tree.
	currentIndex int
	proveIndex   int
	proveSet     [][]byte
}

// A subTree is a sub-tree in the Tree. 'height' refers to how tall the subtree
// is. The children of the tree are not accessible, as they have already been
// hashed into 'value'. 'next' is the next sub-tree, and is guaranteed to have
// a higher height unless it is nil.
type subTree struct {
	next   *subTree
	height int
	value  []byte
}

// sum returns the sha256 hash of the input data.
func sum(h hash.Hash, data []byte) (result []byte) {
	h.Write(data)
	result = h.Sum(nil)
	h.Reset()
	return
}

// New initializes a Tree with a hash object, which is used to hash and combine
// data.
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

// Push adds a leaf to the tree by hashing the data and then inserting the
// result as a leaf.
func (t *Tree) Push(data []byte) {
	// Even before hashing the data, check if the index of this element is
	// equal to the proveIndex, which means we need to save the data as a part
	// of the proveSet.
	if t.currentIndex == t.proveIndex {
		t.proveSet = append(t.proveSet, data)
	}

	// Hash the data, creating a sub-tree of height 1.
	value := sum(t.hash, data)
	height := 1

	// Before inserting the subtree, check the height of the next subtree.
	// While the height of the next subtree is equal to the height of the
	// current tree, hash the two trees together to get a new sub-tree of
	// greater height.
	for t.head != nil && height == t.head.height {
		// Before hasing the subtrees together, check if one of the hashes
		// belongs in the proveSet. This is true any time that the height of
		// each tree is equal to the length of the proveSet.
		if t.head.height == len(t.proveSet) {
			// Either t.head.value or value belongs as the next element of the
			// proveSet. 'proveIndex' is guaranteed to be inside one of the
			// subTrees, and the subTree we want is the other subTree. t.head
			// is the earlier subtree in the set, and is of size
			// 2^(t.head.height-1). The index of the first element of this
			// subtree can be determined with the equation:
			// 		((currentIndex / 2^t.head.height) * 2^t.head.height)
			//
			// The final element can be determined by adding the size to the
			// first element. Then you check if the proveIndex is in that
			// range. If yes, grab 'vaue', otherwise, grab 'head.value'.
			fullSize := int(1 << uint(height))
			fullStart := (t.currentIndex / fullSize) * fullSize
			currentStart := fullStart + (fullSize / 2)
			if t.proveIndex < currentStart {
				t.proveSet = append(t.proveSet, value)
			} else {
				t.proveSet = append(t.proveSet, t.head.value)
			}
		}

		value = sum(t.hash, append(t.head.value, value...))
		height++
		t.head = t.head.next
	}

	// Add the new subtree to the top of the stack.
	t.head = &subTree{
		next:   t.head,
		height: height,
		value:  value,
	}
	t.currentIndex++
}

// Root returns the Merkle root of the data that has been pushed into the Tree.
// Asking for the root when no data has been added will return nil. The tree is
// left unaltered.
func (t *Tree) Root() (root []byte) {
	// If the has never been pushed to, return nil.
	if t.head == nil {
		return nil
	}

	// The root is formed by hashing together sub-trees in order from least in
	// height to greatest in height.
	current := t.head
	root = current.value
	for current.next != nil {
		root = sum(t.hash, append(current.next.value, root...))
		current = current.next
	}
	return root
}

// Prove returns a proof that the data at index 'proveIndex' is an element in
// the current Tree. The proof will be invalid if any more elements are added
// to the tree after calling Prove. Prove does not alter the tree.
func (t *Tree) Prove() (proveSet [][]byte) {
	// Return nil if the Tree is empty.
	if t.head == nil {
		return nil
	}

	// At this point, there will be at least one and perhaps multiple subTrees
	// in the Tree. The current height of the proveSet will be the height of
	// the subTree that the proveIndex is in. There will be exactly one subTree
	// of that height in the proveSet. All of the subTrees of lower heights get
	// hashed together into a single hash, and that hash is grabbed. We need to
	// to add the hashes of all greater sized trees to the proveSet.
	//
	// First loop condenses all of the smaller subTrees and combine them until
	// you get the subTree whose hash you need. Second loop grabs a hash of
	// every larget subTree until they have all been added.
	myHeight := len(t.proveSet)
	current := t.head
	value := current.value
	for current.next != nil {
		if current.next.height == myHeight {
			t.proveSet = append(t.proveSet, value)
			// Skip over the subTree that proveIndex is in.
			current = current.next.next
			break
		}
		value = sum(t.hash, append(current.next.value, value...))
		current = current.next
	}
	for current != nil {
		if myHeight < current.height {
			t.proveSet = append(t.proveSet, current.value)
		}
		current = current.next
	}
	return t.proveSet
}

// VerifyProof takes a merkle, a proveSet, and a proveIndex and returns true if
// the first element of the prove set is a leaf of data in the merkle root.
func VerifyProof(h hash.Hash, merkleRoot []byte, proveSet [][]byte, proveIndex int, numSegments int) bool {
	if numSegments == 0 {
		return true
	}
	if len(proveSet) == 0 || merkleRoot == nil {
		println("header violation")
		return false
	}

	// Determine the size of the largest full subTree (a tree with 2^n leaves)
	// that contains the proveIndex.
	largerSubTrees := 0
	value := sum(h, proveSet[0])
	proveSet = proveSet[1:]
	for {
		// Determine the size of the largest remaining subTree.
		subTreeSize := 1
		for subTreeSize*2 <= numSegments {
			subTreeSize *= 2
		}
		if proveIndex < subTreeSize {
			// We have found the subtree that contains the prove index. Build
			// up the proof inside of the complete subTree, where we don't need
			// to worry about edge cases.
			height := 1
			for int(1<<uint(height)) <= subTreeSize {
				heightSize := int(1 << uint(height))
				heightStart := (proveIndex / heightSize) * heightSize
				mid := heightStart + (heightSize / 2)
				if len(proveSet) == 0 {
					println("mainloop violation")
					return false
				}
				if proveIndex < mid {
					value = sum(h, append(value, proveSet[0]...))
				} else {
					value = sum(h, append(proveSet[0], value...))
				}
				height++
				proveSet = proveSet[1:]
			}

			// Check if there's a smaller subTree.
			if subTreeSize < numSegments {
				if len(proveSet) == 0 {
					println("smaller subtree violation")
					return false
				}
				value = sum(h, append(value, proveSet[0]...))
				proveSet = proveSet[1:]
			}
			break
		}
		largerSubTrees++
		proveIndex -= subTreeSize
		numSegments -= subTreeSize
	}

	// Add for each larger subTree.
	for i := 0; i < largerSubTrees; i++ {
		if len(proveSet) == 0 {
			println("larget subtree violation")
			return false
		}
		value = sum(h, append(proveSet[0], value...))
		proveSet = proveSet[1:]
	}

	// If there are still elements remaining in the prove set, return false.
	if len(proveSet) != 0 {
		println("proveSet remaining violation")
		return false
	}

	if bytes.Compare(value, merkleRoot) == 0 {
		return true
	} else {
		println("endgame violation")
		return false
	}
}
