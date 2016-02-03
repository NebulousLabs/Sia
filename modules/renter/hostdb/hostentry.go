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
	online      bool
}

// insert adds a host entry to the state. The host will be inserted into the
// set of all hosts, and if it is online and responding to requests it will be
// put into the list of active hosts.
//
// TODO: Function should return an error.
func (hdb *HostDB) insertHost(host modules.HostSettings) {
	// Remove garbage hosts and local hosts.
	if !host.NetAddress.IsValid() {
		return
	}
	if host.NetAddress.IsLoopback() && build.Release != "testing" {
		return
	}
	// Don't do anything if we've already seen this host.
	if _, exists := hdb.allHosts[host.NetAddress]; exists {
		return
	}

	// Create hostEntry and add to allHosts.
	h := &hostEntry{
		HostSettings: host,
		reliability:  DefaultReliability,
	}
	hdb.allHosts[host.NetAddress] = h

	// Add the host to the scan queue. If the scan is successful, the host
	// will be placed in activeHosts.
	hdb.scanHostEntry(h)
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
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()

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

// IsOffline reports whether a host is offline. If the HostDB has no record of
// the host, IsOffline will return false and spawn a goroutine to the scan the
// host.
func (hdb *HostDB) IsOffline(addr modules.NetAddress) bool {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()

	if _, ok := hdb.activeHosts[addr]; ok {
		return false
	}
	if h, ok := hdb.allHosts[addr]; ok {
		return !h.online
	}
	// no record of the host; add it to the HostDB
	hdb.insertHost(modules.HostSettings{NetAddress: addr})
	return false
}
