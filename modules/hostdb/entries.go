package hostdb

import (
	"crypto/rand"
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
)

// entryWeight returns the weight of a host entry according to the internal
// metrics of the HostDB. Currently, that means using the weight (10^30 *
// entry.Collateral / (entry.Price)^2), where entry.Collateral is adjusted to
// be at most twice the price and at least half the price.
func entryWeight(entry modules.HostEntry) (weight consensus.Currency) {
	// Clamp the collateral to between 0.5x and 2x the price.
	collateral := entry.Collateral
	if collateral.Cmp(entry.Price.MulFloat(2)) > 0 {
		collateral = entry.Price.MulFloat(2)
	} else if collateral.Cmp(entry.Price.MulFloat(0.5)) < 0 {
		collateral = entry.Price.MulFloat(0.5)
	}

	// Create a baseline weight of 10^30, which adds precision to the equation
	// and makes sure that all reasonable prices end with a weight that's
	// greater than zero.
	weight = consensus.NewCurrency(new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil))
	weight = weight.Mul(collateral).Div(entry.Price).Div(entry.Price)
	return
}

// insertCompleteHostEntry inserts a host entry without making a network call
// to the host to grab the settings.
func (hdb *HostDB) insertCompleteHostEntry(entry *modules.HostEntry) {
	// Active entries are stored by address, sans port number. This limits each
	// IP to advertising 1 host. Do not replace
	hostname := entry.IPAddress.Host()
	_, exists := hdb.activeHosts[hostname]
	if exists {
		return
	}

	// Add the host as a node to the host tree.
	if hdb.hostTree == nil {
		hdb.hostTree = createNode(nil, *entry)
		hdb.activeHosts[hostname] = hdb.hostTree
	} else {
		_, hostNode := hdb.hostTree.insert(*entry)
		hdb.activeHosts[hostname] = hostNode
	}
}

// threadedInsert adds a host entry to the state. The entry is passed by
// pointer so that changes made to the entry are received by all parties.
func (hdb *HostDB) threadedInsert(entry *modules.HostEntry) {
	// Get the settings from the host. Host will remain active if a valid
	// response is not given.
	var hs modules.HostSettings
	err := entry.IPAddress.RPC("HostSettings", nil, &hs)
	if err != nil {
		return
	}

	// Lock the host db after the network call has finished.
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	entry.HostSettings = hs
	hdb.insertCompleteHostEntry(entry)
}

// Remove deletes an entry from the hostdb.
func (hdb *HostDB) remove(addr network.Address) error {
	// Remove the host from the set of all hosts.
	_, exists := hdb.allHosts[addr]
	if exists {
		delete(hdb.allHosts, addr)
	}

	// Strip the port (see insert), then check the set of active hosts for an
	// entry.
	hostname := addr.Host()

	// See if the node is in the set of active hosts.
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
func (hdb *HostDB) FlagHost(addr network.Address) error {
	return hdb.Remove(addr)
}

// Insert is the thread-safe version of insert.
func (hdb *HostDB) Insert(entry modules.HostEntry) error {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	hdb.allHosts[entry.IPAddress] = &entry
	go hdb.threadedInsert(&entry)
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
func (hdb *HostDB) Remove(addr network.Address) error {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	return hdb.remove(addr)
}
