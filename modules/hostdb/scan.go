package hostdb

// scan.go contians the functions which periodically scan the list of all hosts
// to see which hosts are online or offline, and to get any updates to the
// settings of the hosts.

import (
	"crypto/rand"
	"math/big"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	DefaultReliability  = types.NewCurrency64(20)
	InactiveReliability = types.NewCurrency64(10)
	UnreachablePenalty  = types.NewCurrency64(1)
)

// decrementReliability reduces the reliability of a node, moving it out of the
// set of active hosts or deleting it entirely if necessary.
func (hdb *HostDB) decrementReliability(addr modules.NetAddress, penalty types.Currency) {
	// Look up the entry and decrement the reliability.
	entry, exists := hdb.allHosts[addr]
	if !exists {
		return
	}
	entry.Reliability = entry.Reliability.Sub(penalty)

	// If the entry is in the active database and has fallen below
	// InactiveReliability, remove it from the active database.
	node, exists := hdb.activeHosts[addr]
	if exists && entry.Reliability.Cmp(InactiveReliability) < 0 {
		delete(hdb.activeHosts, entry.IPAddress)
		node.removeNode()
	}

	// If the reliability has fallen to 0, remove the host from the
	// database entirely.
	if entry.Reliability.IsZero() {
		delete(hdb.allHosts, addr)
	}
}

// threadedProbeHost tries to fetch the settings of a host. If successful, the
// host is put in the set of active hosts. If unsuccessful, the host id deleted
// from the set of active hosts.
func (hdb *HostDB) threadedProbeHost(entry *modules.HostEntry) {
	// Request the most recent set of settings from the host.
	var settings modules.HostSettings
	err := hdb.gateway.RPC(entry.IPAddress, "HostSettings", func(conn modules.NetConn) error {
		return conn.ReadObject(&settings, 1024)
	})

	// Now that network communicaiton is done, lock the hostdb.
	id := hdb.mu.Lock()
	defer hdb.mu.Unlock(id)
	if err != nil {
		hdb.decrementReliability(entry.IPAddress, UnreachablePenalty)
		return
	}

	// Update the host settings, reliability, and weight.
	entry.HostSettings = settings
	entry.Reliability = DefaultReliability
	entry.Weight = hdb.priceWeight(*entry)

	// If the host is not already in the database, add it to the database.
	_, exists := hdb.activeHosts[entry.IPAddress]
	if !exists {
		hdb.insertNode(entry)
	}
}

// threadedScan is an ongoing function which will query the full set of hosts
// every few hours to see who is online and available for uploading.
func (hdb *HostDB) threadedScan() {
	for {
		// Sleep for a random amount of time between 4 and 24 hours. The time
		// is randomly generated so that hosts who are only on at certain times
		// of the day or week will still be included.
		randSleep, err := rand.Int(rand.Reader, big.NewInt(int64(time.Hour*20)))
		if err != nil {
			if build.DEBUG {
				panic(err)
			} else {
				// If there's an error generating the random number, just sleep
				// for 15 hours because it'll hit all times of the day after
				// enough iterations.
				randSleep = big.NewInt(int64(time.Hour * 15))
			}
		}
		time.Sleep(time.Duration(randSleep.Int64()) + time.Hour*4)

		// Ask every host in the database for settings.
		//
		// TODO: enforce some limit on the number of hosts that will be
		// queried.
		id := hdb.mu.Lock()
		{
			for _, host := range hdb.allHosts {
				go hdb.threadedProbeHost(host)
			}
		}
		hdb.mu.Unlock(id)
	}
}
