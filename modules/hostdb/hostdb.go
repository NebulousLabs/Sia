package hostdb

import (
	"crypto/rand"
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
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
	state       *consensus.State
	recentBlock consensus.BlockID

	hostTree      *hostNode
	activeHosts   map[network.Address]*hostNode
	inactiveHosts map[network.Address]*modules.HostEntry

	mu sync.RWMutex
}

// New returns an empty HostDatabase.
func New(state *consensus.State) (hdb *HostDB, err error) {
	if state == nil {
		err = errors.New("HostDB can't use nil State")
		return
	}
	hdb = &HostDB{
		state:         state,
		recentBlock:   state.CurrentBlock().ID(),
		activeHosts:   make(map[network.Address]*hostNode),
		inactiveHosts: make(map[network.Address]*modules.HostEntry),
	}
	return
}

// insert will add a host entry to the state.
func (hdb *HostDB) insert(entry modules.HostEntry) error {
	_, exists := hdb.activeHosts[entry.IPAddress]
	if exists {
		return errors.New("entry of given id already exists in host db")
	}

	if hdb.hostTree == nil {
		hdb.hostTree = createNode(nil, entry)
		hdb.activeHosts[entry.IPAddress] = hdb.hostTree
	} else {
		_, hostNode := hdb.hostTree.insert(entry)
		hdb.activeHosts[entry.IPAddress] = hostNode
	}
	return nil
}

// Insert adds an entry to the hostdb, wrapping the standard insert call with a
// lock. When called externally, the lock needs to be in place, however
// sometimes insert needs to be called internally when there is already a lock
// in place.
func (hdb *HostDB) Insert(entry modules.HostEntry) error {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	return hdb.insert(entry)
}

func (hdb *HostDB) FlagHost(addr network.Address) error {
	// Check that we're online at all.

	// Remove the flagged host.
	//
	// TODO: Smarter flagging code, perhaps cut the weight for example.
	return hdb.Remove(addr)
}

// Remove deletes an entry from the hostdb.
func (hdb *HostDB) remove(addr network.Address) error {
	// See if the node is in the set of active hosts.
	node, exists := hdb.activeHosts[addr]
	if !exists {
		// If the node is in the set of inactive hosts, delete from that set,
		// otherwise return a not found error.
		_, exists := hdb.inactiveHosts[addr]
		if exists {
			delete(hdb.inactiveHosts, addr)
			return nil
		} else {
			return errors.New("address not found in host database")
		}
	}

	// Delete the node from the active hosts, and remove it from the tree.
	delete(hdb.activeHosts, addr)
	node.remove()

	return nil
}

func (hdb *HostDB) Remove(addr network.Address) error {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	return hdb.remove(addr)
}

// Update throws a bunch of blocks at the hostdb to be integrated.
func (hdb *HostDB) update() (err error) {
	hdb.state.RLock()
	initialStateHeight := hdb.state.Height()
	rewoundBlocks, appliedBlocks, err := hdb.state.BlocksSince(hdb.recentBlock)
	if err != nil {
		// TODO: this may be a serious problem; if recentBlock is not updated,
		// will BlocksSince always return an error?
		hdb.state.RUnlock()
		return
	}
	hdb.recentBlock = hdb.state.CurrentBlock().ID()
	hdb.state.RUnlock()

	// Remove hosts announced in blocks that were rewound.
	for _, blockID := range rewoundBlocks {
		block, exists := hdb.state.Block(blockID)
		if !exists {
			continue
		}
		for _, entry := range findHostAnnouncements(initialStateHeight, block) {
			err = hdb.remove(entry.IPAddress)
			if err != nil {
				return
			}
		}
	}

	// Add hosts announced in blocks that were applied. For security reasons,
	// the announcements themselves do not contain hosting parameters; we must
	// request these separately, using the address given in the announcement.
	// Each such request is made in a separate thread.
	for _, blockID := range appliedBlocks {
		block, exists := hdb.state.Block(blockID)
		if !exists {
			continue
		}
		for _, entry := range findHostAnnouncements(initialStateHeight, block) {
			go hdb.threadedInsertFromAnnouncement(entry)
		}
	}

	return
}

// RandomHost pulls a random host from the hostdb weighted according to
// whatever internal metrics exist within the hostdb.
func (hdb *HostDB) RandomHost() (h modules.HostEntry, err error) {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()

	if len(hdb.activeHosts) == 0 {
		err = errors.New("no hosts found")
		return
	}

	// Get a random number between 0 and state.TotalWeight and then scroll
	// through state.HostList until at least that much weight has been passed.
	randInt, err := rand.Int(rand.Reader, hdb.hostTree.weight.Big())
	if err != nil {
		return
	}
	// no possibility of error
	randWeight, _ := consensus.BigToCurrency(randInt)
	return hdb.hostTree.entryAtWeight(randWeight)
}
