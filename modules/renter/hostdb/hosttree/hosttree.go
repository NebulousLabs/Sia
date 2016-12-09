package hosttree

import (
	"crypto/rand"
	"errors"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// ErrWeightTooHeavy is returned from a Fetch() call if a weight that exceeds
	// the total weight of the tree is requested.
	ErrWeightTooHeavy = errors.New("requested a too-heavy weight")

	// ErrNegativeWeight is returned from an Insert() call if an entry with a
	// negative weight is added to the tree. Entries must always have a positive
	// weight.
	ErrNegativeWeight = errors.New("cannot insert using a negative weight")

	// ErrNilEntry is returned if a fetch call results in a nil tree entry. nodes
	// should always have a non-nil entry, unless they have been Delete()ed.
	ErrNilEntry = errors.New("node has a nil entry")

	// ErrHostExists is returned if an Insert is called with a public key that
	// already exists in the tree.
	ErrHostExists = errors.New("host already exists in the tree")

	// ErrNoSuchHost is returned if Remove is called with a public key that does
	// not exist in the tree.
	ErrNoSuchHost = errors.New("no host with specified public key")
)

type (
	// HostTree is a data structure that contains a weighted tree of
	// `HostEntries`, and provides methods for inserting, retreiving, and
	// modifing entries.
	HostTree struct {
		root *node

		// hosts is a map of public keys to nodes.
		hosts map[string]*node
	}

	// HostEntry is an entry in the host tree.
	HostEntry struct {
		modules.HostDBEntry
		weight types.Currency

		FirstSeen   types.BlockHeight
		Reliability types.Currency
	}

	// node is a node in the tree.
	node struct {
		parent *node
		left   *node
		right  *node

		count int  // cumulative count of this node and all children
		taken bool // `taken` indicates whether there is an active host at this node or not.

		weight types.Currency
		entry  *HostEntry
	}
)

// createNode creates a new node using the provided `parent` and `entry`.
func createNode(parent *node, entry *HostEntry) *node {
	return &node{
		parent: parent,
		weight: entry.weight,
		count:  1,

		taken: true,
		entry: entry,
	}
}

// New creates a new, empty, HostTree.
func New() *HostTree {
	return &HostTree{
		root: &node{
			count: 1,
		},
		hosts: make(map[string]*node),
	}
}

// recursiveInsert inserts an entry into the appropriate place in the tree. The
// running time of recursiveInsert is log(n) in the maximum number of elements
// that have ever been in the tree.
func (n *node) recursiveInsert(entry *HostEntry) (nodesAdded int, newnode *node) {
	// If there is no parent and no children, and the node is not taken, assign
	// this entry to this node.
	if n.parent == nil && n.left == nil && n.right == nil && !n.taken {
		n.entry = entry
		n.taken = true
		n.weight = entry.weight
		newnode = n
		return
	}

	n.weight = n.weight.Add(entry.weight)

	// If the current node is empty, add the entry but don't increase the
	// count.
	if !n.taken {
		n.taken = true
		n.entry = entry
		newnode = n
		return
	}

	// Insert the element into the lest populated side.
	if n.left == nil {
		n.left = createNode(n, entry)
		nodesAdded = 1
		newnode = n.left
	} else if n.right == nil {
		n.right = createNode(n, entry)
		nodesAdded = 1
		newnode = n.right
	} else if n.left.count <= n.right.count {
		nodesAdded, newnode = n.left.recursiveInsert(entry)
	} else {
		nodesAdded, newnode = n.right.recursiveInsert(entry)
	}

	n.count += nodesAdded
	return
}

// nodeAtWeight grabs an element in the tree that appears at the given weight.
// Though the tree has an arbitrary sorting, a sufficiently random weight will
// pull a random element. The tree is searched through in a post-ordered way.
func (n *node) nodeAtWeight(weight types.Currency) (*node, error) {
	// Sanity check - weight must be less than the total weight of the tree.
	if weight.Cmp(n.weight) > 0 {
		return nil, ErrWeightTooHeavy
	}

	// Check if the left or right child should be returned.
	if n.left != nil {
		if weight.Cmp(n.left.weight) < 0 {
			return n.left.nodeAtWeight(weight)
		}
		weight = weight.Sub(n.left.weight) // Search from the 0th index of the right side.
	}
	if n.right != nil && weight.Cmp(n.right.weight) < 0 {
		return n.right.nodeAtWeight(weight)
	}

	// Should we panic here instead?
	if !n.taken {
		return nil, ErrNilEntry
	}

	// Return the root entry.
	return n, nil
}

// remove takes a node and removes it from the tree by climbing through the
// list of parents. remove does not delete nodes.
func (n *node) remove() {
	n.weight = n.weight.Sub(n.entry.weight)
	n.taken = false
	current := n.parent
	for current != nil {
		current.weight = current.weight.Sub(n.entry.weight)
		current = current.parent
	}
}

// Insert inserts the entry provided to `entry` into the host tree. Insert will
// return an error if the input host already exists.
func (ht *HostTree) Insert(entry *HostEntry) error {
	if _, exists := ht.hosts[string(entry.PublicKey.Key)]; exists {
		return ErrHostExists
	}

	_, node := ht.root.recursiveInsert(entry)

	ht.hosts[string(entry.PublicKey.Key)] = node
	return nil
}

// Remove removes the host with the public key provided by `pk`.
func (ht *HostTree) Remove(pk types.SiaPublicKey) error {
	node, exists := ht.hosts[string(pk.Key)]
	if !exists {
		return ErrNoSuchHost
	}
	node.remove()
	delete(ht.hosts, string(pk.Key))

	return nil
}

// Modify updates a host entry at the given public key, replacing the old entry
// with the entry provided by `newEntry`.
func (ht *HostTree) Modify(entry *HostEntry) error {
	node, exists := ht.hosts[string(entry.PublicKey.Key)]
	if !exists {
		return ErrNoSuchHost
	}

	node.remove()
	_, node = ht.root.recursiveInsert(entry)

	ht.hosts[string(entry.PublicKey.Key)] = node
	return nil
}

// Fetch grabs a random `n` hosts from the tree. There will be no repeats, but
// the length of the slice returned may be less than `n`, and may even be zero.
// The hosts that are returned first have the higher priority. Hosts passed to
// `ignore` will not be considered; pass `nil` if no blacklist is desired.
func (ht *HostTree) Fetch(n int, ignore []types.SiaPublicKey) ([]modules.HostDBEntry, error) {
	var hosts []modules.HostDBEntry
	var removedEntries []*HostEntry

	for _, pubkey := range ignore {
		node, exists := ht.hosts[string(pubkey.Key)]
		if !exists {
			continue
		}
		node.remove()
		delete(ht.hosts, string(pubkey.Key))
		removedEntries = append(removedEntries, node.entry)
	}

	for len(hosts) < n && len(ht.hosts) > 0 {
		randWeight, err := rand.Int(rand.Reader, ht.root.weight.Big())
		if err != nil {
			return hosts, err
		}
		node, err := ht.root.nodeAtWeight(types.NewCurrency(randWeight))
		if err != nil {
			return hosts, err
		}

		if node.entry.HostDBEntry.AcceptingContracts {
			hosts = append(hosts, node.entry.HostDBEntry)
		}

		removedEntries = append(removedEntries, node.entry)
		node.remove()
		delete(ht.hosts, string(node.entry.PublicKey.Key))
	}

	for _, entry := range removedEntries {
		err := ht.Insert(entry)
		if err != nil {
			return hosts, err
		}
	}

	return hosts, nil
}
