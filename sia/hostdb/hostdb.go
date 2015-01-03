package hostdb

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
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

	_, hostNode := hdb.hostTree.insert(&entry)
	hdb.activeHosts[entry.ID] = hostNode
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

func findHostAnnouncements(height consensus.BlockHeight, b consensus.Block) (entries []HostEntry, err error) {
	for _, t := range b.Transactions {
		// Check the arbitrary data of the transaction to fill out the host database.
		if len(t.ArbitraryData) == 0 {
			continue
		}
		if len(t.ArbitraryData[0]) < 8 {
			continue
		}

		dataIndicator := encoding.DecUint64([]byte(t.ArbitraryData[0][0:8]))
		if dataIndicator == 1 {
			var ha HostAnnouncement
			err = encoding.Unmarshal([]byte(t.ArbitraryData[0][8:]), &ha)
			if err != nil {
				return
			}

			// Verify that the host has declared values that are relevant to our
			// interests.
			if ha.SpendConditions.CoinAddress() != t.Outputs[ha.FreezeIndex].SpendHash {
				continue
			}
			if ha.MinChallengeWindow > 100 {
				continue
			}
			if ha.MinTolerance > 10 {
				continue
			}
			freeze := consensus.Currency(ha.SpendConditions.TimeLock-height) * t.Outputs[ha.FreezeIndex].Value
			if freeze <= 0 {
				continue
			}

			// Add the host to the host database.
			entryID := t.OutputID(int(ha.FreezeIndex))
			entries = append(entries, HostEntry{
				ID:          string(entryID[:]),
				IPAddress:   ha.IPAddress,
				MinFilesize: ha.MinFilesize,
				MaxFilesize: ha.MaxFilesize,
				MinDuration: ha.MinDuration,
				MaxDuration: ha.MaxDuration,
				Window:      ha.MinChallengeWindow,
				Tolerance:   ha.MinTolerance,
				Price:       ha.Price,
				Burn:        ha.Burn,
				Freeze:      freeze,
				CoinAddress: ha.CoinAddress,
			})
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
