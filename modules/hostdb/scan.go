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

const (
	DefaultReliability  = 20
	InactiveReliability = 10
	UnreachablePenalty  = 1
)

// threadedProbeHost tries to fetch the settings of a host. If successful, the
// host is put in the set of active hosts. If unsuccessful, the host id deleted
// from the set of active hosts.
//
// TODO LOG: Log what happens in this function.
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
		// Beacuse there was an error, decrement the reliability.
		entry.Reliability = entry.Reliability.Sub(types.NewCurrency64(UnreachablePenalty))

		// If the reliability has fallen below InactiveReliability, remove the host from the list
		// of active hosts.
		node, exists := hdb.activeHosts[entry.IPAddress]
		if exists && entry.Reliability.Cmp(types.NewCurrency64(InactiveReliability)) < 0 {
			delete(hdb.activeHosts, entry.IPAddress)
			node.removeNode()
		}

		// If the reliability has fallen to 0, remove the host from the
		// database entirely.
		if entry.Reliability.IsZero() {
			delete(hdb.allHosts, entry.IPAddress)
		}
		return
	}

	// Update the host settings, reliability, and weight.
	entry.HostSettings = settings
	entry.Reliability = types.NewCurrency64(DefaultReliability)
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
