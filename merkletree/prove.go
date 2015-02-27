package merkletree

import (
	"hash"
)

type TreeProve struct {
	head *node
	hash hash.Hash

	provingForIndex int
	currentIndex    int
	proveSet        [][]byte
}

func NewTreeProve(h hash.Hash, index int) *TreeProve {
	return &TreeProve{
		hash:            h,
		provingForIndex: index,
	}
}

func (t *TreeProve) SetIndex(index int) {
	t.head = nil
	t.provingForIndex = index
	t.currentIndex = 0
	t.proveSet = nil
}

func (t *TreeProve) Push(data []byte) {
	if t.currentIndex == t.provingForIndex {
		t.proveSet = append(t.proveSet, data)
	}

	value := sum(t.hash, data)
	height := 1
	for t.head != nil && height == t.head.height {
		if t.head.height == len(t.proveSet) && height == len(t.proveSet) {
			arenaSize := int(1 << uint(len(t.proveSet)))
			arenaStart := (t.currentIndex / arenaSize) * arenaSize
			forwardStart := arenaStart + arenaSize/2
			if t.provingForIndex < forwardStart {
				t.proveSet = append(t.proveSet, value)
			} else {
				t.proveSet = append(t.proveSet, t.head.value)
			}
		}

		value = sum(t.hash, append(t.head.value, value...))
		height++
		t.head = t.head.next
	}

	t.head = &node{
		next:   t.head,
		height: height,
		value:  value,
	}
	t.currentIndex++
}

func (t *TreeProve) Prove() (root []byte, proveSet [][]byte) {
	if t.head == nil {
		return
	}
	myHeight := len(t.proveSet)

	value := t.head.value
	for t.head.next != nil {
		if t.head.next.height == myHeight {
			t.proveSet = append(t.proveSet, t.head.value)
		}
		if t.head.height > myHeight {
			t.proveSet = append(t.proveSet, t.head.value)
		}
		value = sum(t.hash, append(t.head.next.value, value...))
		t.head = t.head.next
	}
	if t.head.height > myHeight {
		t.proveSet = append(t.proveSet, t.head.value)
	}

	root = value
	proveSet = t.proveSet
	t.head = nil
	t.currentIndex = 0
	t.proveSet = nil
	return
}
