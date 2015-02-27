package merkletree

import (
	"hash"
)

// TODO: Explain how and why this works here. Why + how should not be part of
// the docstring of a function, that should only explain 'what'. Why and how
// belong elsewhere.

// A Tree builds a Merkle tree. Add data one slice at a time, and the tree will
// hash the data and build out the Merkle tree using log(n) memory, where n is
// the number of times 'Push' is called.
type Tree struct {
	head *node
	hash hash.Hash
}

// A node is a memeber of the Tree.
type node struct {
	next   *node
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

// Push adds a leaf to the tree by hashing the data and then inserting the
// result as a leaf.
func (t *Tree) Push(data []byte) {
	value := sum(t.hash, data)
	height := 1
	for t.head != nil && height == t.head.height {
		value = sum(t.hash, append(t.head.value, value...))
		height++
		t.head = t.head.next
	}

	t.head = &node{
		next:   t.head,
		height: height,
		value:  value,
	}
}

// Root returns the Merkle root of the data that has been pushed into the Tree,
// then clears all of the data out. Making a copy of the tree is sufficient to
// preserve the data. Asking for the root when no data has been added will
// return nil.
func (t *Tree) Root() (root []byte) {
	if t.head == nil {
		return
	}

	value := t.head.value
	for t.head.next != nil {
		value = sum(t.hash, append(t.head.next.value, value...))
		t.head = t.head.next
	}

	t.head = nil
	root = value
	return
}
