package hostdb

import (
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

// insert inserts a host entry into the node.
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
