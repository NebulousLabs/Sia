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
	DefaultScanSleep = 15 * time.Hour
	MaxScanSleep     = 20 * time.Hour
	MinScanSleep     = 4 * time.Hour

	MaxActiveHosts              = 200
	InactiveHostCheckupQuantity = 100
)

var (
	ActiveReliability   = types.NewCurrency64(20)
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
	entry.reliability = entry.reliability.Sub(penalty)

	// If the entry is in the active database and has fallen below
	// InactiveReliability, remove it from the active database.
	node, exists := hdb.activeHosts[addr]
	if exists && entry.reliability.Cmp(InactiveReliability) < 0 {
		delete(hdb.activeHosts, entry.IPAddress)
		node.removeNode()
		hdb.notifySubscribers()
	}

	// If the reliability has fallen to 0, remove the host from the
	// database entirely.
	if entry.reliability.IsZero() {
		delete(hdb.allHosts, addr)
	}
}

// threadedProbeHost tries to fetch the settings of a host. If successful, the
// host is put in the set of active hosts. If unsuccessful, the host id deleted
// from the set of active hosts.
func (hdb *HostDB) threadedProbeHost(entry *hostEntry) {
	// Request the most recent set of settings from the host.
	var settings modules.HostSettings
	err := hdb.gateway.RPC(entry.IPAddress, "HostSettings", func(conn modules.NetConn) error {
		maxSettingsLen := uint64(1024)
		return conn.ReadObject(&settings, maxSettingsLen)
	})

	// Now that network communicaiton is done, lock the hostdb to modify the
	// host entry.
	id := hdb.mu.Lock()
	defer hdb.mu.Unlock(id)
	if err != nil {
		hdb.decrementReliability(entry.IPAddress, UnreachablePenalty)
		return
	}

	// Update the host settings, reliability, and weight. The old IPAddress
	// must be preserved.
	settings.IPAddress = entry.HostSettings.IPAddress
	entry.HostSettings = settings
	entry.reliability = ActiveReliability
	entry.weight = hdb.hostWeight(*entry)

	// If the host is not already in the database and 'MaxActiveHosts' has not
	// been reached, add the host to the database.
	_, exists := hdb.activeHosts[entry.IPAddress]
	if !exists && len(hdb.activeHosts) < MaxActiveHosts {
		hdb.insertNode(entry)
		hdb.notifySubscribers()
	}
}

// threadedScan is an ongoing function which will query the full set of hosts
// every few hours to see who is online and available for uploading.
func (hdb *HostDB) threadedScan() {
	for {
		// Determine who to scan. At most 'MaxActiveHosts' will be scanned,
		// starting with the active hosts followed by a random selection of the
		// inactive hosts.
		id := hdb.mu.Lock()
		{
			// Check all of the active hosts.
			for _, host := range hdb.activeHosts {
				go hdb.threadedProbeHost(host.hostEntry)
			}

			// Assemble all of the inactive hosts into a single array and
			// shuffle it.
			i := 0
			random := make([]*hostEntry, len(hdb.allHosts))
			for _, entry := range hdb.allHosts {
				random[i] = &entry
				i++
			}

			// Randomize the slice by swapping each element with an element
			// that hasn't been visited yet.
			for i := 0; i < len(hdb.allHosts); i++ {
				N, err := rand.Int(rand.Reader, big.NewInt(int64(len(hdb.allHosts)-i)))
				if err != nil {
					if build.DEBUG {
						panic(err)
					}
				} else {
					break
				}

				n := int(N.Int64()) + i
				tmp := random[i]
				random[i] = random[n]
				random[n] = tmp
			}

			// Select the first InactiveHostCheckupQuantity hosts from the
			// shuffled list.
			n := InactiveHostCheckupQuantity
			if len(random) < InactiveHostCheckupQuantity {
				n = len(random)
			}
			for i := 0; i < n; i++ {
				go hdb.threadedProbeHost(random[i])
			}
		}
		hdb.mu.Unlock(id)

		// Sleep for a random amount of time between 4 and 24 hours. The time
		// is randomly generated so that hosts who are only on at certain times
		// of the day or week will still be included. Random times also make it
		// harder for hosts to game the system.
		randSleep, err := rand.Int(rand.Reader, big.NewInt(int64(MaxScanSleep)))
		if err != nil {
			if build.DEBUG {
				panic(err)
			} else {
				// If there's an error generating the random number, just sleep
				// for 15 hours because it'll hit all times of the day after
				// enough iterations.
				randSleep = big.NewInt(int64(DefaultScanSleep))
			}
		}
		time.Sleep(time.Duration(randSleep.Int64()) + MinScanSleep)
	}
}
