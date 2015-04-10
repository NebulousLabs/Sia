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

// threadedProbeHost tries to fetch the settings of a host. If successful, the
// host is put in the set of active hosts. If unsuccessful, the host id deleted
// from the set of active hosts.
func (hdb *HostDB) threadedProbeHost(entry *modules.HostEntry) {
	// Request the most recent set of settings from the host.
	var settings modules.HostSettings
	err := hdb.gateway.RPC(entry.IPAddress, "HostSettings", func(conn modules.NetConn) error {
		return conn.ReadObject(&settings, 1024)
	})
	id := hdb.mu.Lock()
	defer hdb.mu.Unlock(id)
	if err != nil {
		// Decrement the reliability.
		entry.Reliability = entry.Reliability.Sub(types.NewCurrency64(1))

		// If the reliability has fallen below 5, remove the host from the list
		// of active hosts.
		node, exists := hdb.activeHosts[entry.IPAddress]
		if exists && entry.Reliability.Cmp(types.NewCurrency64(5)) < 0 {
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

	// Re-insert the host into the database as it is online and has responded
	// with a set of settings.
	entry.HostSettings = settings
	entry.Reliability = types.NewCurrency64(10)
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
		// Sleep for a random amount of time between 4 and 24 hours. The time is randomly generated
		randSleep, err := rand.Int(rand.Reader, big.NewInt(int64(time.Hour*20)))
		if err != nil {
			if build.DEBUG {
				panic(err)
			} else {
				randSleep = big.NewInt(int64(time.Hour * 13))
			}
		}
		time.Sleep(time.Duration(randSleep.Int64()) + time.Hour*4)
	}
}
