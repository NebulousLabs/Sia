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
	hostEntry *hostEntry
}

// createNode makes a new node the fill a host entry.
func createNode(parent *hostNode, entry *hostEntry) *hostNode {
	return &hostNode{
		parent: parent,
		weight: entry.weight,
		count:  1,

		taken:     true,
		hostEntry: entry,
	}
}

// nodeAtWeight grabs an element in the tree that appears at the given weight.
// Though the tree has an arbitrary sorting, a sufficiently random weight will
// pull a random element. The tree is searched through in a post-ordered way.
func (hn *hostNode) nodeAtWeight(weight types.Currency) (*hostNode, error) {
	// Sanity check - weight must be less than the total weight of the tree.
	if weight.Cmp(hn.weight) > 0 {
		return nil, ErrOverweight
	}

	// Check if the left or right child should be returned.
	if hn.left != nil {
		if weight.Cmp(hn.left.weight) < 0 {
			return hn.left.nodeAtWeight(weight)
		}
		weight = weight.Sub(hn.left.weight) // Search from 0th index of right side.
	}
	if hn.right != nil && weight.Cmp(hn.right.weight) < 0 {
		return hn.right.nodeAtWeight(weight)
	}

	// Sanity check
	if build.DEBUG {
		if !hn.taken {
			panic("should not be returning a nil entry")
		}
	}

	// Return the root entry.
	return hn, nil
}

// recursiveInsert is a recurisve function for adding a hostNode to an existing tree
// of hostNodes. The first call should always be on hostdb.hostTree. Running
// time of recursiveInsert is log(n) in the maximum number of elements that have
// ever been in the tree.
func (hn *hostNode) recursiveInsert(entry *hostEntry) (nodesAdded int, newNode *hostNode) {
	hn.weight = hn.weight.Add(entry.weight)

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

// insertNode inserts a host entry into the host tree, removing
// any conflicts. The host settings are assummed to be correct. Though hosts
// with 0 weight will never be selected, they are accepted into the tree.
func (hdb *HostDB) insertNode(entry *hostEntry) {
	// If there's already a host of the same id, remove that host.
	priorEntry, exists := hdb.activeHosts[entry.NetAddress]
	if exists {
		priorEntry.removeNode()
	}

	// Insert the updated entry into the host tree.
	if hdb.hostTree == nil {
		hdb.hostTree = createNode(nil, entry)
		hdb.activeHosts[entry.NetAddress] = hdb.hostTree
	} else {
		_, hostNode := hdb.hostTree.recursiveInsert(entry)
		hdb.activeHosts[entry.NetAddress] = hostNode
	}
}

// remove takes a node and removes it from the tree by climbing through the
// list of parents. remove does not delete nodes.
func (hn *hostNode) removeNode() {
	hn.weight = hn.weight.Sub(hn.hostEntry.weight)
	hn.taken = false
	current := hn.parent
	for current != nil {
		current.weight = current.weight.Sub(hn.hostEntry.weight)
		current = current.parent
	}
}

// isEmpty returns whether the hostTree contains no entries.
func (hdb *HostDB) isEmpty() bool {
	return hdb.hostTree == nil || hdb.hostTree.weight.IsZero()
}

// RandomHosts will pull up to 'n' random hosts from the hostdb. There will be
// no repeats, but the length of the slice returned may be less than 'n', and
// may even be 0. The hosts that get returned first have the higher priority.
// Hosts specified in 'ignore' will not be considered; pass 'nil' if no
// blacklist is desired.
func (hdb *HostDB) RandomHosts(n int, ignore []modules.NetAddress) (hosts []modules.HostExternalSettings) {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	if hdb.isEmpty() {
		return
	}

	// These will be restored after selection is finished.
	var removedEntries []*hostEntry

	// Remove hosts that we want to ignore.
	for _, addr := range ignore {
		node, exists := hdb.activeHosts[addr]
		if !exists {
			continue
		}
		node.removeNode()
		delete(hdb.activeHosts, addr)
		removedEntries = append(removedEntries, node.hostEntry)
	}

	// Pick a host, remove it from the tree, and repeat until we have n hosts
	// or the tree is empty.
	for len(hosts) < n && !hdb.isEmpty() {
		randWeight, err := rand.Int(rand.Reader, hdb.hostTree.weight.Big())
		if err != nil {
			break
		}
		node, err := hdb.hostTree.nodeAtWeight(types.NewCurrency(randWeight))
		if err != nil {
			break
		}
		hosts = append(hosts, node.hostEntry.HostExternalSettings)

		node.removeNode()
		delete(hdb.activeHosts, node.hostEntry.NetAddress)
		removedEntries = append(removedEntries, node.hostEntry)
	}

	// Add back all of the entries that got removed.
	for i := range removedEntries {
		hdb.insertNode(removedEntries[i])
	}
	return hosts
}
