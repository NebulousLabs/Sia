package hostdb

import (
	"fmt"
	"github.com/NebulousLabs/Sia/consensus"
)

// hostNode is the node of an unsorted, balanced, weighted binary tree. When
// inserting elements, elements are inserted on the side of the tree with the
// fewest elements. When removing, the node is just made empty but the tree is
// not reorganized.
type hostNode struct {
	parent *hostNode
	weight consensus.Currency // cumulative weight of this node and all children.
	count  int                // cumulative count of all children.

	left  *hostNode
	right *hostNode

	hostEntry *HostEntry
}

// createNode makes a new node the fill a host entry.
func createNode(parent *hostNode, entry *HostEntry) *hostNode {
	return &hostNode{
		parent:    parent,
		weight:    entry.Weight(),
		count:     1,
		hostEntry: entry,
	}
}

// insert inserts a host entry into the node. insert is recursive. The value
// returned is the number of nodes added to the tree, always 1 or 0.
func (hn *hostNode) insert(entry *HostEntry) (nodesAdded int, newNode *hostNode) {
	hn.weight += entry.Weight()

	// If the current node is empty, add the entry but don't increase the
	// count.
	if hn.hostEntry == nil {
		hn.hostEntry = entry
		newNode = hn
		return
	}

	// Insert the element into the lightest side.
	if hn.left == nil {
		hn.left = createNode(hn, entry)
		nodesAdded = 1
		newNode = hn.left
	} else if hn.right == nil {
		hn.right = createNode(hn, entry)
		nodesAdded = 1
		newNode = hn.right
	} else if hn.left.weight < hn.right.weight {
		nodesAdded, newNode = hn.left.insert(entry)
	} else {
		nodesAdded, newNode = hn.right.insert(entry)
	}

	hn.count += nodesAdded
	return
}

// remove takes a node and removes it from the tree by climbing through the
// list of parents. Remove does not delete nodes.
func (hn *hostNode) remove() {
	hn.weight -= hn.hostEntry.Weight()
	current := hn.parent
	for current != nil {
		current.weight -= hn.hostEntry.Weight()
		current = current.parent
	}
}

// entryAtWeight grabs an element in the tree that appears at the given
// weight. Though the tree has an arbitrary sorting, a sufficiently random
// weight will pull a random element. The tree is searched through in a
// post-ordered way.
func (hn *hostNode) entryAtWeight(weight consensus.Currency) (entry HostEntry, err error) {
	// Check for an errored weight call.
	if weight > hn.weight {
		err = fmt.Errorf("tree is not that heavy, asked for %v and got %v", weight, hn.weight)
		return
	}

	// Check if the left or right child should be returned.
	if hn.left != nil {
		if weight < hn.left.weight {
			return hn.left.entryAtWeight(weight)
		}
		weight -= hn.left.weight // Search from 0th index of right side.
	}
	if hn.right != nil && weight < hn.right.weight {
		return hn.right.entryAtWeight(weight)
	}

	// Sanity check
	if hn.hostEntry == nil {
		panic("should not be returning a nil entry")
	}

	// Return the root entry.
	entry = *hn.hostEntry
	return
}
