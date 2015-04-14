package hostdb

// weightedlist.go manages a weighted list of nodes that can be queried
// randomly. The functions for inserting, removing, and fetching nodes from the
// list are housed in this file.

import (
	"crypto/rand"
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrOverweight = errors.New("requested a too-heavy weight")
)

// hostNode is the node of an unsorted, balanced, weighted binary tree. When
// inserting elements, elements are inserted on the side of the tree with the
// fewest elements. When removing, the node is just made empty but the tree is
// not reorganized. The size of the tree will never decrease, but it will also
// not increase unless it has more entries than it has ever had before.
type hostNode struct {
	parent *hostNode
	count  int // Cumulative count of this node and  all children.

	// Currently the only weight supported is priceWeight. Eventually, support
	// will be added for multiple tunable types of weight. The different
	// weights all represent the cumulative weight of this node and all
	// children.
	weight types.Currency

	left  *hostNode
	right *hostNode

	taken     bool // Indicates whether there is an active host at this node or not.
	hostEntry modules.HostEntry
}

// createNode makes a new node the fill a host entry.
func createNode(parent *hostNode, entry modules.HostEntry) *hostNode {
	return &hostNode{
		parent: parent,
		weight: entry.Weight,
		count:  1,

		taken:     true,
		hostEntry: entry,
	}
}

// entryAtWeight grabs an element in the tree that appears at the given
// weight. Though the tree has an arbitrary sorting, a sufficiently random
// weight will pull a random element. The tree is searched through in a
// post-ordered way.
func (hn *hostNode) entryAtWeight(weight types.Currency) (entry modules.HostEntry, err error) {
	// Sanity check - entryAtWeight should never be called with a too-large
	// weight.
	if weight.Cmp(hn.weight) > 0 {
		err = ErrOverweight
		return
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

// recursiveInsert is a recurisve function for adding a hostNode to an existing tree
// of hostNodes. The first call should always be on hostdb.hostTree. Running
// time of recursiveInsert is log(n) in the maximum number of elements that have
// ever been in the tree.
func (hn *hostNode) recursiveInsert(entry modules.HostEntry) (nodesAdded int, newNode *hostNode) {
	hn.weight = hn.weight.Add(entry.Weight)

	// If the current node is empty, add the entry but don't increase the
	// count.
	if !hn.taken {
		hn.taken = true
		hn.hostEntry = entry
		newNode = hn
		return
	}

	// Insert the element into the lest populated side.
	if hn.left == nil {
		hn.left = createNode(hn, entry)
		nodesAdded = 1
		newNode = hn.left
	} else if hn.right == nil {
		hn.right = createNode(hn, entry)
		nodesAdded = 1
		newNode = hn.right
	} else if hn.left.count < hn.right.count {
		nodesAdded, newNode = hn.left.recursiveInsert(entry)
	} else {
		nodesAdded, newNode = hn.right.recursiveInsert(entry)
	}

	hn.count += nodesAdded
	return
}

// insertCompleteHostEntry inserts a host entry into the host tree, removing
// any conflicts. The host settings are assummed to be correct. Though hosts
// with 0 weight will never be selected, they are accetped into the tree.
func (hdb *HostDB) insertNode(entry *modules.HostEntry) {
	// If there's already a host of the same id, remove that host.
	priorEntry, exists := hdb.activeHosts[entry.IPAddress]
	if exists {
		priorEntry.removeNode()
	}

	// Insert the updated entry into the host tree.
	if hdb.hostTree == nil {
		hdb.hostTree = createNode(nil, *entry)
		hdb.activeHosts[entry.IPAddress] = hdb.hostTree
	} else {
		_, hostNode := hdb.hostTree.recursiveInsert(*entry)
		hdb.activeHosts[entry.IPAddress] = hostNode
	}
}

// remove takes a node and removes it from the tree by climbing through the
// list of parents. remove does not delete nodes.
func (hn *hostNode) removeNode() {
	hn.weight = hn.weight.Sub(hn.hostEntry.Weight)
	hn.taken = false
	current := hn.parent
	for current != nil {
		current.weight = current.weight.Sub(hn.hostEntry.Weight)
		current = current.parent
	}
}

// RandomHost pulls a random host from the hostdb weighted according to the
// internal metrics of the hostdb.
func (hdb *HostDB) RandomHost() (entry modules.HostEntry, err error) {
	id := hdb.mu.Lock()
	defer hdb.mu.Unlock(id)

	if len(hdb.activeHosts) == 0 {
		err = errors.New("no hosts found")
		return
	}

	// Get a random number between 0 and state.TotalWeight and then scroll
	// through state.HostList until at least that much weight has been passed.
	randWeight, err := rand.Int(rand.Reader, hdb.hostTree.weight.Big())
	if err != nil {
		return
	}
	return hdb.hostTree.entryAtWeight(types.NewCurrency(randWeight))
}
