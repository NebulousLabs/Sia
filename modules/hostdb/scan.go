package hostdb

// scan.go contians the functions which periodically scan the list of all hosts
// to see which hosts are online or offline, and to get any updates to the
// settings of the hosts.

import (
	"crypto/rand"
	"math/big"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	DefaultScanSleep = 2*time.Hour + 18*time.Minute
	MaxScanSleep     = 6 * time.Hour
	MinScanSleep     = 1 * time.Hour

	MaxActiveHosts              = 500
	InactiveHostCheckupQuantity = 250

	maxSettingsLen = 1024

	hostRequestTimeout = 5 * time.Second

	// scanningThreads is the number of threads that will be probing hosts for
	// their settings and checking for reliability.
	scanningThreads = 25
)

var (
	MaxReliability     = types.NewCurrency64(50) // Given the scanning defaults, about 1 week of survival.
	DefaultReliability = types.NewCurrency64(20) // Given the scanning defaults, about 3 days of survival.
	UnreachablePenalty = types.NewCurrency64(1)
)

// addHostToScanPool creates a gofunc that adds a host to the scan pool. If the
// scan pool is currently full, the blocking gofunc will not cause a deadlock.
// The gofunc is created inside of this function to eliminate the burden of
// needing to remember to call 'go addHostToScanPool'.
func (hdb *HostDB) scanHostEntry(entry *hostEntry) {
	go func() {
		hdb.scanPool <- entry
	}()
}

// decrementReliability reduces the reliability of a node, moving it out of the
// set of active hosts or deleting it entirely if necessary.
func (hdb *HostDB) decrementReliability(addr modules.NetAddress, penalty types.Currency) {
	// Look up the entry and decrement the reliability.
	entry, exists := hdb.allHosts[addr]
	if !exists {
		return
	}
	entry.reliability = entry.reliability.Sub(penalty)

	// If the entry is in the active database, remove it from the active
	// database.
	node, exists := hdb.activeHosts[addr]
	if exists {
		delete(hdb.activeHosts, entry.IPAddress)
		node.removeNode()
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
func (hdb *HostDB) threadedProbeHosts() {
	for hostEntry := range hdb.scanPool {
		// Request settings from the queued host entry.
		var settings modules.HostSettings
		err := func() error {
			conn, err := net.DialTimeout("tcp", string(hostEntry.IPAddress), hostRequestTimeout)
			if err != nil {
				return err
			}
			defer conn.Close()
			err = encoding.WriteObject(conn, modules.RPCSettings)
			if err != nil {
				return err
			}
			return encoding.ReadObject(conn, &settings, maxSettingsLen)
		}()

		// Now that network communication is done, lock the hostdb to modify the
		// host entry.
		id := hdb.mu.Lock()
		{
			if err != nil {
				hdb.decrementReliability(hostEntry.IPAddress, UnreachablePenalty)
				hdb.mu.Unlock(id)
				continue
			}

			// Update the host settings, reliability, and weight. The old IPAddress
			// must be preserved.
			settings.IPAddress = hostEntry.HostSettings.IPAddress
			hostEntry.HostSettings = settings
			hostEntry.reliability = MaxReliability
			hostEntry.weight = calculateHostWeight(*hostEntry)

			// If the host is not already in the database and 'MaxActiveHosts' has not
			// been reached, add the host to the database.
			_, exists1 := hdb.activeHosts[hostEntry.IPAddress]
			_, exists2 := hdb.allHosts[hostEntry.IPAddress]
			if !exists1 && exists2 && len(hdb.activeHosts) < MaxActiveHosts {
				hdb.insertNode(hostEntry)
			}
		}
		hdb.mu.Unlock(id)
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
			// Scan all active hosts.
			for _, host := range hdb.activeHosts {
				hdb.scanHostEntry(host.hostEntry)
			}

			// Assemble all of the inactive hosts into a single array.
			var random []*hostEntry
			for _, entry := range hdb.allHosts {
				entry2, exists := hdb.activeHosts[entry.IPAddress]
				if !exists {
					random = append(random, entry)
				} else {
					if build.DEBUG {
						if entry2.hostEntry != entry {
							panic("allHosts + activeHosts mismatch!")
						}
					}
				}
			}

			// Randomize the slice by swapping each element with an element
			// that hasn't been visited yet.
			for i := 0; i < len(random); i++ {
				N, err := rand.Int(rand.Reader, big.NewInt(int64(len(random)-i)))
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
			// shuffled list and scan them.
			n := InactiveHostCheckupQuantity
			if len(random) < InactiveHostCheckupQuantity {
				n = len(random)
			}
			for i := 0; i < n; i++ {
				hdb.scanHostEntry(random[i])
			}
		}
		hdb.mu.Unlock(id)

		// Sleep for a random amount of time before doing another round of
		// scanning. The minimums and maximums keep the scan time reasonable,
		// while the randomness prevents the scanning from always happening at
		// the same time of day or week.
		maxBig := big.NewInt(int64(MaxScanSleep))
		minBig := big.NewInt(int64(MinScanSleep))
		randSleep, err := rand.Int(rand.Reader, maxBig.Sub(maxBig, minBig))
		if err != nil {
			if build.DEBUG {
				panic(err)
			} else {
				// If there's an error, sleep for the default amount of time.
				defaultBig := big.NewInt(int64(DefaultScanSleep))
				randSleep = defaultBig.Sub(defaultBig, minBig)
			}
		}
		time.Sleep(time.Duration(randSleep.Int64()) + MinScanSleep) // this means the MaxScanSleep is actual Max+Min.
	}
}
