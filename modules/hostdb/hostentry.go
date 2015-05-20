package hostdb

import (
	"math/big"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// Because most weights would otherwise be fractional, we set the base
	// weight to 10^80 to give ourselves lots of precision when determing the
	// weight of a host
	baseWeight = types.NewCurrency(new(big.Int).Exp(big.NewInt(10), big.NewInt(120), nil))
)

type hostEntry struct {
	modules.HostSettings
	weight      types.Currency
	reliability types.Currency
}

// hostWeight returns the weight of a host according to the settings of the
// host database. Currently, only the price is considered.
func (hdb *HostDB) hostWeight(entry hostEntry) (weight types.Currency) {
	// Prevent a divide by zero error by making sure the price is at least one.
	price := entry.Price
	if price.Cmp(types.NewCurrency64(0)) <= 0 {
		price = types.NewCurrency64(1)
	}

	// Divide the base weight by the cube of the price.
	return baseWeight.Div(price).Div(price).Div(price)
}

// insert adds a host entry to the state. The host will be inserted into the
// set of all hosts, and if it is online and responding to requests it will be
// put into the list of active hosts.
func (hdb *HostDB) insertHost(host modules.HostSettings) {
	// Add the host to allHosts.
	entry := &hostEntry{
		HostSettings: host,
		reliability:  InactiveReliability,
	}
	_, exists := hdb.allHosts[entry.IPAddress]
	if !exists {
		hdb.allHosts[entry.IPAddress] = entry
		go hdb.threadedProbeHost(entry)
	}
}

// Remove deletes an entry from the hostdb.
func (hdb *HostDB) removeHost(addr modules.NetAddress) error {
	delete(hdb.allHosts, addr)

	// See if the node is in the set of active hosts.
	node, exists := hdb.activeHosts[addr]
	if exists {
		delete(hdb.activeHosts, addr)
		node.removeNode()
		hdb.notifySubscribers()
	}

	return nil
}

// ActiveHosts returns the hosts that can be randomly selected out of the
// hostdb.
func (hdb *HostDB) ActiveHosts() (activeHosts []modules.HostSettings) {
	id := hdb.mu.RLock()
	defer hdb.mu.RUnlock(id)

	for _, node := range hdb.activeHosts {
		activeHosts = append(activeHosts, node.hostEntry.HostSettings)
	}
	return
}

// AllHosts returns all of the hosts known to the hostdb, including the
// inactive ones.
func (hdb *HostDB) AllHosts() (allHosts []modules.HostSettings) {
	id := hdb.mu.RLock()
	defer hdb.mu.RUnlock(id)

	for _, entry := range hdb.allHosts {
		allHosts = append(allHosts, entry.HostSettings)
	}
	return
}

// InsertHost inserts a host into the database.
func (hdb *HostDB) InsertHost(host modules.HostSettings) error {
	id := hdb.mu.Lock()
	defer hdb.mu.Unlock(id)
	hdb.insertHost(host)
	return nil
}

// RemoveHost removes a host from the database.
func (hdb *HostDB) RemoveHost(addr modules.NetAddress) error {
	id := hdb.mu.Lock()
	defer hdb.mu.Unlock(id)
	return hdb.removeHost(addr)
}
