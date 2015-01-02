package hostdb

import (
	// "crypto/rand"
	// "errors"
	// "math/big"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	// "github.com/NebulousLabs/Sia/encoding"
	// "github.com/NebulousLabs/Sia/sia"
)

// Need to be easily able to swap hosts in and out of an active and inactive
// list.
type HostDatabase struct {
	activeHosts   *hostNode
	inactiveHosts map[string]*hostNode
	sync.RWMutex
}

// New returns an empty HostDatabase.
func New() (hdb *HostDatabase) {
	hdb = &HostDatabase{
		inactiveHosts: make(map[string]*hostNode),
	}
	return
}

// TODO: Implement this.
func (hdb *HostDatabase) Info() ([]byte, error) {
	return nil, nil
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
			err = hdb.Remove(entry)
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
			return
		}
		if len(t.ArbitraryData[0]) < 8 {
			return
		}

		dataIndicator := encoding.DecUint64([]byte(t.ArbitraryData[0][0:8]))
		if dataIndicator == 1 {
			var ha HostAnnouncement
			err := encoding.Unmarshal([]byte(t.ArbitraryData[0][8:]), &ha)
			if err != nil {
				return
			}

			// Verify that the host has declared values that are relevant to our
			// interests.
			if ha.SpendConditions.CoinAddress() != t.Outputs[ha.FreezeIndex].SpendHash {
				return
			}
			if ha.MinChallengeWindow > 100 {
				return
			}
			if ha.MinTolerance > 10 {
				return
			}
			freeze := consensus.Currency(ha.SpendConditions.TimeLock-e.state.Height()) * t.Outputs[ha.FreezeIndex].Value
			if freeze <= 0 {
				return
			}

			// Add the host to the host database.
			he = HostEntry{
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
			}
		}
	}
	return
}

/*
// ChooseHost orders the hosts by weight and picks one at random.
func (hdb *HostDatabase) ChooseHost() (h HostEntry, err error) {
	if len(hdb.HostList) == 0 {
		err = errors.New("no hosts found")
		return
	}
	if hdb.TotalWeight == 0 {
		panic("state has 0 total weight but not 0 length host list?")
	}

	// Get a random number between 0 and state.TotalWeight and then scroll
	// through state.HostList until at least that much weight has been passed.
	randInt, err := rand.Int(rand.Reader, big.NewInt(int64(hdb.TotalWeight)))
	if err != nil {
		return
	}
	randWeight := consensus.Currency(randInt.Int64())
	weightPassed := consensus.Currency(0)
	var i int
	for i = 0; randWeight > weightPassed; i++ {
		weightPassed += hdb.HostList[i].Weight()
	}
	i -= 1

	h = hdb.HostList[i]
	return
}
*/
