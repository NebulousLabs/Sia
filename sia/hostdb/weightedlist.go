package hostdb

import (
	"errors"
	"github.com/NebulousLabs/Sia/consensus"
)

// hostNode is the node of an unsorted, balanced, weighted binary tree.
type hostNode struct {
	parent *hostNode
	weight consensus.Currency // cumulative weight of this node and all children.
	count  int                // cumulative count of all children.

	left  *hostNode
	right *hostNode

	hostEntry *HostEntry
}

// createNode makes a new node the fill a host entry.
func (hn *hostNode) createNode(entry *HostEntry) *hostNode {
	return &hostNode{
		parent:    hn,
		weight:    entry.Weight(),
		count:     1,
		hostEntry: entry,
	}
}

// insert inserts a host entry into the node. insert is recursive.
func (hn *hostNode) insert(entry *HostEntry) {
	if hn.left == nil {
		hn.left = hn.createNode(entry)
	} else if hn.right == nil {
		hn.right = hn.createNode(entry)
	}

	if hn.left.weight < hn.right.weight {
		hn.left.insert(entry)
	} else {
		hn.right.insert(entry)
	}

	hn.count++
	hn.weight += entry.Weight()
}

// remove takes a node and removes it from the tree by climbing through the
// list of parents.
func (hn *hostNode) remove() {
	prev := hn
	current := hn.parent
	for current != nil {
		if current.left == prev {
			current.left = nil
		} else if current.right == prev {
			current.right = nil
		} else {
			panic("malformed tree!")
		}

		current.count--
		current.weight -= hn.hostEntry.Weight()
		prev = current
		current = current.parent
	}
}

// elementAtWeight grabs an element in the tree that appears at the given
// weight. Though the tree has an arbitrary sorting, a sufficiently random
// weight will pull a random element. The tree is searched through in a
// post-ordered way.
func (hn *hostNode) elementAtWeight(weight consensus.Currency) (entry HostEntry, err error) {
	// Check for an errored weight call.
	if weight > hn.weight {
		err = errors.New("tree is not that heavy")
		return
	}

	// Check if the left or right child should be returned.
	if hn.left != nil {
		if hn.left.weight > weight {
			return hn.left.elementAtWeight(weight)
		}
		weight -= hn.left.weight
	}
	if hn.right != nil {
		return hn.right.elementAtWeight(weight)
	}

	// Return the root entry.
	entry = *hn.hostEntry
	return
}
