package hostdb

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// A hostEntry represents a host on the network.
type hostEntry struct {
	modules.HostSettings
	weight      types.Currency
	reliability types.Currency
}

// insert adds a host entry to the state. The host will be inserted into the
// set of all hosts, and if it is online and responding to requests it will be
// put into the list of active hosts.
//
// TODO: Function should return an error.
func (hdb *HostDB) insertHost(host modules.HostSettings) {
	// Remove garbage hosts and local hosts.
	if !host.IPAddress.IsValid() {
		return
	}
	if host.IPAddress.IsLoopback() && build.Release != "testing" {
		return
	}

	// Add the host to allHosts.
	entry := &hostEntry{
		HostSettings: host,
		reliability:  DefaultReliability,
	}
	_, exists := hdb.allHosts[entry.IPAddress]
	if !exists {
		hdb.allHosts[entry.IPAddress] = entry
		hdb.scanHostEntry(entry)
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
	}

	return nil
}

// ActiveHosts returns the hosts that can be randomly selected out of the
// hostdb.
func (hdb *HostDB) ActiveHosts() (activeHosts []modules.HostSettings) {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()

	for _, node := range hdb.activeHosts {
		activeHosts = append(activeHosts, node.hostEntry.HostSettings)
	}
	return
}

// AllHosts returns all of the hosts known to the hostdb, including the
// inactive ones.
func (hdb *HostDB) AllHosts() (allHosts []modules.HostSettings) {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()

	for _, entry := range hdb.allHosts {
		allHosts = append(allHosts, entry.HostSettings)
	}
	return
}

// AveragePrice returns the average price of a host.
func (hdb *HostDB) AveragePrice() types.Currency {
	// maybe a more sophisticated way of doing this
	var totalPrice types.Currency
	sampleSize := 18
	hosts := hdb.randomHosts(sampleSize, nil)
	if len(hosts) == 0 {
		return totalPrice
	}
	for _, host := range hosts {
		totalPrice = totalPrice.Add(host.Price)
	}
	return totalPrice.Div(types.NewCurrency64(uint64(len(hosts))))
}
