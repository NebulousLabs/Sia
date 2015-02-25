package hostdb

import (
	"crypto/rand"
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	// Because most weights would otherwise be fractional, we set the base
	// weight to 10^30 to give ourselves lots of precision when determing an
	// entries weight.
	baseWeight = consensus.NewCurrency(new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil))

	// Convenience variables for doing currency math. Originally we were just
	// using MulFloat but this was causing precision problems during testing.
	// The actual functionality of the program isn't affected by loss of
	// precision, it just makes testing simpler.
	currencyZero     = consensus.NewCurrency64(0)
	currencyOne      = consensus.NewCurrency64(1)
	currencyTwo      = consensus.NewCurrency64(2)
	currencyFive     = consensus.NewCurrency64(5)
	currencyTen      = consensus.NewCurrency64(10)
	currencyTwenty   = consensus.NewCurrency64(20)
	currencyThousand = consensus.NewCurrency64(1e3)
)

// entryWeight returns the weight of an entry according to the price and
// collateral of the entry. The current general equation is:
//		(collateral / price^2)
//
// The collateral is clamped so that it is not treated as being less than 0.5x
// the price or more than 2x the price.
func entryWeight(entry modules.HostEntry) (weight consensus.Currency) {
	// Clamp the collateral to between 0.5x and 2x the price.
	collateral := entry.Collateral
	if collateral.Cmp(entry.Price.Mul(currencyTwo)) > 0 {
		collateral = entry.Price.Mul(currencyTwo)
	} else if collateral.Cmp(entry.Price.Div(currencyTwo)) < 0 {
		collateral = entry.Price.Div(currencyTwo)
	}

	// Prevent a divide by zero error by making sure the price is at least one.
	price := entry.Price
	if price.Cmp(currencyZero) <= 0 {
		price = currencyOne
	}

	// Take the base weight, multiply it by the clapmed collateral, then divide
	// it by the square of the price.
	return baseWeight.Mul(collateral).Div(price).Div(price)
}

// insertCompleteHostEntry inserts a host entry into the host tree, removing
// any conflicts. The host settings are assummed to be correct.
func (hdb *HostDB) insertCompleteHostEntry(entry *modules.HostEntry) {
	// If there's already a host of the same id, remove that host.
	hostname := entry.IPAddress.Host()
	priorEntry, exists := hdb.activeHosts[hostname]
	if exists {
		priorEntry.remove()
	}

	// Insert the updated entry into the host tree.
	if hdb.hostTree == nil {
		hdb.hostTree = createNode(nil, *entry)
		hdb.activeHosts[hostname] = hdb.hostTree
	} else {
		_, hostNode := hdb.hostTree.insert(*entry)
		hdb.activeHosts[hostname] = hostNode
	}
}

// insertActiveHost takes a host entry and queries the host for its settings.
// Once it has the settings, it inserts it into the host tree. If it cannot get
// the settings, it gives up and quits.
func (hdb *HostDB) threadedInsertActiveHost(entry *modules.HostEntry) {
	// Get the settings from the host. Host is removed from the set of active
	// hosts if no response is given.
	var hs modules.HostSettings
	err := entry.IPAddress.RPC("HostSettings", nil, &hs)
	if err != nil {
		return
	}

	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	entry.HostSettings = hs

	hdb.insertCompleteHostEntry(entry)
}

// insert adds a host entry to the state. The host is guaranteed to make it
// into the set of all hosts. The host will only make it into the set of active
// hosts if there are no previous hosts that exist at the same ip address (this
// is to make it more difficult for a single host to sybil the network). If the
// host is at the same ip address and port number as the existing host, then
// it's assumed to be the same host, and an update is made.
//
// Once the entry has been added to the database, all calls use pointers to the
// entry. This is because some of the calls modify the entry (under a lock) and
// everyone needs to receive the modifications.
func (hdb *HostDB) insert(entry modules.HostEntry) {
	// Add the host to allHosts.
	hdb.allHosts[entry.IPAddress] = &entry

	// Check if there is another host in the set of active hosts with the same
	// ip address. If there is, this host is not given precedent. The exception
	// is if this host has the same full address (including port number), in
	// which case it's assumed that the host is trying to post an update.
	hostname := entry.IPAddress.Host()
	priorEntry, exists := hdb.activeHosts[hostname]
	if exists {
		if priorEntry.hostEntry.IPAddress != entry.IPAddress {
			return
		}
	}

	go hdb.threadedInsertActiveHost(&entry)
}

// Remove deletes an entry from the hostdb.
func (hdb *HostDB) remove(addr network.Address) error {
	delete(hdb.allHosts, addr)

	// See if the node is in the set of active hosts.
	hostname := addr.Host()
	node, exists := hdb.activeHosts[hostname]
	if exists {
		delete(hdb.activeHosts, hostname)
		node.remove()
	}

	return nil
}

// FlagHost is called when a host is caught misbehaving. In general, the
// behavior is that the host will be called less often. For the time being,
// that means removing the host from the database outright.
func (hdb *HostDB) FlagHost(addr modules.NetAddress) error {
	return hdb.Remove(addr)
}

// Insert attempts to insert a host entry into the database.
func (hdb *HostDB) Insert(entry modules.HostEntry) error {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	hdb.insert(entry)
	return nil
}

// NumHosts returns the number of hosts in the active database.
func (hdb *HostDB) NumHosts() int {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()
	hdb.update()

	if hdb.hostTree == nil {
		return 0
	}
	return hdb.hostTree.count
}

// RandomHost pulls a random host from the hostdb weighted according to the
// internal metrics of the hostdb.
func (hdb *HostDB) RandomHost() (h modules.HostEntry, err error) {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()
	hdb.update()

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
	return hdb.hostTree.entryAtWeight(consensus.NewCurrency(randWeight))
}

// Remove is the thread-safe version of remove.
func (hdb *HostDB) Remove(addr modules.NetAddress) error {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	return hdb.remove(addr)
}
