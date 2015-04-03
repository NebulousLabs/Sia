package hostdb

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// hostNode is the node of an unsorted, balanced, weighted binary tree. When
// inserting elements, elements are inserted on the side of the tree with the
// fewest elements. When removing, the node is just made empty but the tree is
// not reorganized.
type hostNode struct {
	parent *hostNode
	weight types.Currency // cumulative weight of this node and all children.
	count  int            // cumulative count of all children.

	left  *hostNode
	right *hostNode

	taken     bool // Used because modules.HostEntry can't be compared to nil.
	hostEntry modules.HostEntry
}

// createNode makes a new node the fill a host entry.
func createNode(parent *hostNode, entry modules.HostEntry) *hostNode {
	return &hostNode{
		parent: parent,
		weight: entryWeight(entry),
		count:  1,

		taken:     true,
		hostEntry: entry,
	}
}

// insert inserts a host entry into the node. insert is recursive. The value
// returned is the number of nodes added to the tree, always 1 or 0.
func (hn *hostNode) insert(entry modules.HostEntry) (nodesAdded int, newNode *hostNode) {
	hn.weight = hn.weight.Add(entryWeight(entry))

	// If the current node is empty, add the entry but don't increase the
	// count.
	if !hn.taken {
		hn.taken = true
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
	} else if hn.left.weight.Cmp(hn.right.weight) < 0 {
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
	hn.weight = hn.weight.Sub(entryWeight(hn.hostEntry))
	hn.taken = false
	current := hn.parent
	for current != nil {
		current.weight = current.weight.Sub(entryWeight(hn.hostEntry))
		current = current.parent
	}
}

// entryAtWeight grabs an element in the tree that appears at the given
// weight. Though the tree has an arbitrary sorting, a sufficiently random
// weight will pull a random element. The tree is searched through in a
// post-ordered way.
func (hn *hostNode) entryAtWeight(weight types.Currency) (entry modules.HostEntry, err error) {
	// Sanity check - entryAtWeight should never be called with a too-large
	// weight.
	if build.DEBUG {
		if weight.Cmp(hn.weight) > 0 {
			panic("entryAtWeight called with an input exceeding the size of the database.")
		}
	}

	// Check if the left or right child should be returned.
	if hn.left != nil {
		if weight.Cmp(hn.left.weight) < 0 {
			return hn.left.entryAtWeight(weight)
		}
		weight = weight.Sub(hn.left.weight) // Search from 0th index of right side.
	}
	if hn.right != nil && weight.Cmp(hn.right.weight) < 0 {
		return hn.right.entryAtWeight(weight)
	}

	// Sanity check
	if build.DEBUG {
		if !hn.taken {
			panic("should not be returning a nil entry")
		}
	}

	// Return the root entry.
	entry = hn.hostEntry
	return
}
