package hostdb

import (
	// "crypto/rand"
	// "errors"
	// "math/big"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	// "github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/network"
	// "github.com/NebulousLabs/Sia/sia"
)

// Need to be easily able to swap hosts in and out of an active and inactive
// list.
type HostDatabase struct {
	activeHosts   *hostNode
	inactiveHosts map[string]*hostNode
	sync.RWMutex
}

// A HostAnnouncement is a struct that can appear in the arbitrary data field.
// It is preceded by 8 bytes that decode to the integer 1.
type HostAnnouncement struct {
	IPAddress          network.Address
	TotalStorage       int64 // Can go negative.
	MinFilesize        uint64
	MaxFilesize        uint64
	MinDuration        consensus.BlockHeight
	MaxDuration        consensus.BlockHeight
	MinChallengeWindow consensus.BlockHeight
	MaxChallengeWindow consensus.BlockHeight
	MinTolerance       uint64
	Price              consensus.Currency
	Burn               consensus.Currency
	CoinAddress        consensus.CoinAddress // Host may want to give different addresses to each client.

	SpendConditions consensus.SpendConditions
	FreezeIndex     uint64 // The index of the output that froze coins.
}

// New returns an empty HostDatabase.
func New() (hdb *HostDatabase) {
	hdb = &HostDatabase{
		inactiveHosts: make(map[string]*hostNode),
	}
	return
}

/*
// pullHostEntryFromArbitraryData is one of the most cleverly named functions
// in the Galaxy. Any attempt to ridicule such a glorious name will result in
// immediate deprication of all clothes. Needs to be under a hostdb and state
// lock.
func (e *Core) pullHostEntryFromTransaction(t consensus.Transaction) (he HostEntry, foundAHostEntry bool) {
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

	foundAHostEntry = true
	return
}

// scanAndApplyHosts looks at the arbitrary data of a transaction and adds any
// hosts to the host database. Needs to be under a hostdb and state lock.
func (e *Core) updateHostDB(rewoundBlocks []consensus.BlockID, appliedBlocks []consensus.BlockID) {
	// Remove hosts found in blocks that were rewound. Because the hostdb is
	// like a stack, you can just pop the hosts and be certain that they are
	// the same hosts.
	for _, bid := range rewoundBlocks {
		b, err := e.state.BlockFromID(bid)
		if err != nil {
			panic(err)
		}

		for _, t := range b.Transactions {
			hostEntry, found := e.pullHostEntryFromTransaction(t)
			if !found {
				continue
			}

			e.hostDatabase.HostList = e.hostDatabase.HostList[:len(e.hostDatabase.HostList)-1]
			e.hostDatabase.TotalWeight -= hostEntry.Weight()
		}
	}

	// Add hosts found in blocks that were applied.
	for _, bid := range appliedBlocks {
		b, err := e.state.BlockFromID(bid)
		if err != nil {
			panic(err)
		}

		for _, t := range b.Transactions {
			hostEntry, found := e.pullHostEntryFromTransaction(t)
			if !found {
				continue
			}

			// Add the weight of the host to the total weight of the hosts in
			// the host database.
			e.hostDatabase.HostList = append(e.hostDatabase.HostList, hostEntry)
			e.hostDatabase.TotalWeight += hostEntry.Weight()
		}
	}
}

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
