package hostdb

import (
	"crypto/rand"
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
)

// For the time being, all entries are weighted equally.
//
// TODO: Weight entries according to cryptographically verified queries of host
// capacity.
func entryWeight(entry modules.HostEntry) consensus.Currency {
	return consensus.NewCurrency64(1)
}

// insert adds a host entry to the state.
func (hdb *HostDB) insert(entry modules.HostEntry) error {
	// Entries are stored by address, sans port number. This limits each IP to
	// advertising 1 host.
	hostname := entry.IPAddress.Host()
	_, exists := hdb.activeHosts[hostname]
	if exists {
		return errors.New("entry of given id already exists in host db")
	}

	if hdb.hostTree == nil {
		hdb.hostTree = createNode(nil, entry)
		hdb.activeHosts[hostname] = hdb.hostTree
	} else {
		_, hostNode := hdb.hostTree.insert(entry)
		hdb.activeHosts[hostname] = hostNode
	}
	return nil
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
	return hdb.insert(entry)
}

// NumHosts returns the number of hosts in the active database.
func (hdb *HostDB) NumHosts() int {
	hdb.threadedUpdate()
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()

	if hdb.hostTree == nil {
		return 0
	}
	return hdb.hostTree.count
}

// RandomHost pulls a random host from the hostdb weighted according to the
// internal metrics of the hostdb.
func (hdb *HostDB) RandomHost() (h modules.HostEntry, err error) {
	hdb.threadedUpdate()
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()

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
