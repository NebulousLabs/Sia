package hostdb

import (
	"bytes"
	"fmt"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// A hostEntry represents a host on the network.
type hostEntry struct {
	modules.HostDBEntry

	FirstSeen   types.BlockHeight
	Weight      types.Currency
	Reliability types.Currency
}

// insertHost adds a host entry to the state. The host will be inserted into
// the set of all hosts, and if it is online and responding to requests it will
// be put into the list of active hosts.
//
// TODO: Function should return an error.
func (hdb *HostDB) insertHost(host modules.HostDBEntry) {
	// Remove garbage hosts and local hosts (but allow local hosts in testing).
	if err := host.NetAddress.IsValid(); err != nil {
		hdb.log.Debugf("WARN: host '%v' has an invalid NetAddress: %v", host.NetAddress, err)
		return
	}
	// Don't do anything if we've already seen this host and the public key is
	// the same.
	if knownHost, exists := hdb.allHosts[host.NetAddress]; exists && bytes.Equal(host.PublicKey.Key, knownHost.PublicKey.Key) {
		return
	}

	host.FirstSeen = hdb.blockHeight

	// Create hostEntry and add to allHosts.
	h := &hostEntry{
		HostDBEntry: host,
		Reliability: DefaultReliability,
	}
	hdb.allHosts[host.NetAddress] = h

	// Add the host to the scan queue. If the scan is successful, the host
	// will be placed in activeHosts.
	hdb.queueHostEntry(h)
}

// Remove deletes an entry from the hostdb.
func (hdb *HostDB) removeHost(addr modules.NetAddress) error {
	// See if the node is in the set of active hosts.
	entry, exists := hdb.activeHosts[addr]
	if exists {
		hdb.hostTree.Remove(entry.HostDBEntry.PublicKey)
		delete(hdb.activeHosts, addr)
	}

	// Remove the node from all hosts.
	delete(hdb.allHosts, addr)

	return nil
}

// Host returns the HostSettings associated with the specified NetAddress. If
// no matching host is found, Host returns false.
func (hdb *HostDB) Host(addr modules.NetAddress) (modules.HostDBEntry, bool) {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	entry, ok := hdb.allHosts[addr]
	if !ok || entry == nil {
		return modules.HostDBEntry{}, false
	}
	return entry.HostDBEntry, true
}

// ActiveHosts returns the hosts that can be randomly selected out of the
// hostdb, sorted by preference.
func (hdb *HostDB) ActiveHosts() (activeHosts []modules.HostDBEntry) {
	hdb.mu.RLock()
	numHosts := len(hdb.activeHosts)
	hdb.mu.RUnlock()

	// Get the hosts using RandomHosts so that they are in sorted order.
	sortedHosts, err := hdb.hostTree.SelectRandom(numHosts, nil)
	if err != nil {
		// TODO: handle this error
	}
	return sortedHosts
}

// AllHosts returns all of the hosts known to the hostdb, including the
// inactive ones.
func (hdb *HostDB) AllHosts() (allHosts []modules.HostDBEntry) {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()

	for _, entry := range hdb.allHosts {
		allHosts = append(allHosts, entry.HostDBEntry)
	}
	return
}

// AverageContractPrice returns the average price of a host.
func (hdb *HostDB) AverageContractPrice() types.Currency {
	// maybe a more sophisticated way of doing this
	var totalPrice types.Currency
	sampleSize := 18
	hosts, err := hdb.hostTree.SelectRandom(sampleSize, nil)
	if err != nil {
		fmt.Println(err)
		// TODO: handle this error
	}
	if len(hosts) == 0 {
		return totalPrice
	}
	for _, host := range hosts {
		totalPrice = totalPrice.Add(host.ContractPrice)
	}
	return totalPrice.Div64(uint64(len(hosts)))
}
