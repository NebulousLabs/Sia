package hostdb

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/sia/components"
)

// TODO: Add a whole set of features to the host database that allow hosts to
// be pulled according to a variety of different weights. A 'natural
// preference' will allow users to manually favor certain hosts, but even still
// things that matter are price, burn, perhaps some sort of reliability metric,
// a latency metric, and a throughput metric, as well as perhaps a cooperation
// metric. Some of these need to be added to the HostEntry object, but some of
// them can be polled regularly and managed entirely from within the hostdb.

// The HostDB is a set of hosts that get weighted and inserted into a tree
type HostDB struct {
	hostTree      *hostNode
	activeHosts   map[string]*hostNode
	inactiveHosts map[string]*components.HostEntry

	rwLock sync.RWMutex
}

// New returns an empty HostDatabase.
func New() (hdb *HostDB) {
	hdb = &HostDB{
		activeHosts:   make(map[string]*hostNode),
		inactiveHosts: make(map[string]*components.HostEntry),
	}
	return
}

// insert will add a host entry to the state.
func (hdb *HostDB) insert(entry components.HostEntry) error {
	_, exists := hdb.activeHosts[entry.ID]
	if exists {
		return errors.New("entry of given id already exists in host db")
	}

	if hdb.hostTree == nil {
		hdb.hostTree = createNode(nil, entry)
		hdb.activeHosts[entry.ID] = hdb.hostTree
	} else {
		_, hostNode := hdb.hostTree.insert(entry)
		hdb.activeHosts[entry.ID] = hostNode
	}
	return nil
}

// Insert adds an entry to the hostdb, wrapping the standard insert call with a
// lock. When called externally, the lock needs to be in place, however
// sometimes insert needs to be called internally when there is already a lock
// in place.
func (hdb *HostDB) Insert(entry components.HostEntry) error {
	hdb.lock()
	defer hdb.unlock()
	return hdb.insert(entry)
}

// Remove deletes an entry from the hostdb.
func (hdb *HostDB) Remove(id string) error {
	hdb.lock()
	defer hdb.unlock()

	// See if the node is in the set of active hosts.
	node, exists := hdb.activeHosts[id]
	if !exists {
		// If the node is in the set of inactive hosts, delete from that set,
		// otherwise return a not found error.
		_, exists := hdb.inactiveHosts[id]
		if exists {
			delete(hdb.inactiveHosts, id)
			return nil
		} else {
			return errors.New("id not found in host database")
		}
	}

	// Delete the node from the active hosts, and remove it from the tree.
	delete(hdb.activeHosts, id)
	node.remove()

	return nil
}

// Update throws a bunch of blocks at the hostdb to be integrated.
func (hdb *HostDB) Update(initialStateHeight consensus.BlockHeight, rewoundBlocks []consensus.Block, appliedBlocks []consensus.Block) (err error) {
	hdb.lock()
	defer hdb.unlock()

	// Remove hosts found in blocks that were rewound. Because the hostdb is
	// like a stack, you can just pop the hosts and be certain that they are
	// the same hosts.
	for _, b := range rewoundBlocks {
		var entries []components.HostEntry
		entries, err = findHostAnnouncements(initialStateHeight, b)
		if err != nil {
			return
		}

		for _, entry := range entries {
			err = hdb.Remove(entry.ID)
			if err != nil {
				return
			}
		}
	}

	// Add hosts found in blocks that were applied.
	for _, b := range appliedBlocks {
		var entries []components.HostEntry
		entries, err = findHostAnnouncements(initialStateHeight, b)
		if err != nil {
			return
		}

		for _, entry := range entries {
			err = hdb.insert(entry)
			if err != nil {
				return
			}
		}
	}

	return
}

// RandomHost pulls a random host from the hostdb weighted according to
// whatever internal metrics exist within the hostdb.
func (hdb *HostDB) RandomHost() (h components.HostEntry, err error) {
	hdb.rLock()
	defer hdb.rUnlock()
	if len(hdb.activeHosts) == 0 {
		err = errors.New("no hosts found")
		return
	}

	// Get a random number between 0 and state.TotalWeight and then scroll
	// through state.HostList until at least that much weight has been passed.
	randInt, err := rand.Int(rand.Reader, big.NewInt(int64(hdb.hostTree.weight)))
	if err != nil {
		return
	}
	randWeight := consensus.Currency(randInt.Int64())
	return hdb.hostTree.entryAtWeight(randWeight)
}
