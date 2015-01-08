package hostdb

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
)

// Need to be easily able to swap hosts in and out of an active and inactive
// list.
type HostDatabase struct {
	hostTree      *hostNode
	activeHosts   map[string]*hostNode
	inactiveHosts map[string]*HostEntry
	sync.RWMutex
}

// New returns an empty HostDatabase.
func New() (hdb *HostDatabase) {
	hdb = &HostDatabase{
		activeHosts:   make(map[string]*hostNode),
		inactiveHosts: make(map[string]*HostEntry),
	}
	return
}

func (hdb *HostDatabase) Info() ([]byte, error) {
	return nil, nil
}

func (hdb *HostDatabase) Insert(entry HostEntry) error {
	_, exists := hdb.activeHosts[entry.ID]
	if exists {
		return errors.New("entry of given id already exists in host db")
	}

	if hdb.hostTree == nil {
		hdb.hostTree = createNode(nil, &entry)
		hdb.activeHosts[entry.ID] = hdb.hostTree
	} else {
		_, hostNode := hdb.hostTree.insert(&entry)
		hdb.activeHosts[entry.ID] = hostNode
	}
	return nil
}

func (hdb *HostDatabase) Remove(id string) error {
	node, exists := hdb.activeHosts[id]
	if !exists {
		_, exists := hdb.inactiveHosts[id]
		if exists {
			delete(hdb.inactiveHosts, id)
			return nil
		} else {
			return errors.New("id not found in host database")
		}
	}
	delete(hdb.activeHosts, id)
	node.remove()
	return nil
}

func (hdb *HostDatabase) Update(initialStateHeight consensus.BlockHeight, rewoundBlocks []consensus.Block, appliedBlocks []consensus.Block) (err error) {
	// Remove hosts found in blocks that were rewound. Because the hostdb is
	// like a stack, you can just pop the hosts and be certain that they are
	// the same hosts.
	for _, b := range rewoundBlocks {
		var entries []HostEntry
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
		var entries []HostEntry
		entries, err = findHostAnnouncements(initialStateHeight, b)
		if err != nil {
			return
		}

		for _, entry := range entries {
			err = hdb.Insert(entry)
			if err != nil {
				return
			}
		}
	}

	return
}

func (hdb *HostDatabase) RandomHost() (h HostEntry, err error) {
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
