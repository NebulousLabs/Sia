package merkletree

import (
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

// A Tree builds a Merkle tree. Add data one slice at a time, and the tree will
// hash the data and build out the Merkle tree using log(n) memory, where n is
// the number of times 'Push' is called.
type Tree struct {
	head *subTree
	hash hash.Hash
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
}

// Push adds a leaf to the tree by hashing the data and then inserting the
// result as a leaf.
func (t *Tree) Push(data []byte) {
	// First hash the data, creating a sub-tree of height 1.
	value := sum(t.hash, data)
	height := 1

	// Before inserting the subtree, check the height of the next subtree.
	// While the height of the next subtree is equal to the height of the
	// current tree, hash the two trees together to get a new sub-tree of
	// greater height.
	for t.head != nil && height == t.head.height {
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
	return
}
