package hostdb

// scan.go contains the functions which periodically scan the list of all hosts
// to see which hosts are online or offline, and to get any updates to the
// settings of the hosts.

import (
	"crypto/rand"
	"math/big"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// oldHostSettings is the HostSettings type used prior to v0.5.0. It is
// preserved for compatibility with those hosts.
// COMPATv0.4.8
type oldHostSettings struct {
	NetAddress   modules.NetAddress
	TotalStorage int64
	MinFilesize  uint64
	MaxFilesize  uint64
	MinDuration  types.BlockHeight
	MaxDuration  types.BlockHeight
	WindowSize   types.BlockHeight
	Price        types.Currency
	Collateral   types.Currency
	UnlockHash   types.UnlockHash
}

const (
	DefaultScanSleep = 1*time.Hour + 37*time.Minute
	MaxScanSleep     = 4 * time.Hour
	MinScanSleep     = 1 * time.Hour

	MaxActiveHosts              = 500
	InactiveHostCheckupQuantity = 250

	maxSettingsLen = 2e3

	hostRequestTimeout = 5 * time.Second

	// scanningThreads is the number of threads that will be probing hosts for
	// their settings and checking for reliability.
	scanningThreads = 25
)

var (
	MaxReliability     = types.NewCurrency64(225) // Given the scanning defaults, about 3 weeks of survival.
	DefaultReliability = types.NewCurrency64(75)  // Given the scanning defaults, about 1 week of survival.
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
	entry.online = false

	// If the entry is in the active database, remove it from the active
	// database.
	node, exists := hdb.activeHosts[addr]
	if exists {
		delete(hdb.activeHosts, entry.NetAddress)
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
			conn, err := hdb.dialer.DialTimeout(hostEntry.NetAddress, hostRequestTimeout)
			if err != nil {
				return err
			}
			defer conn.Close()
			err = encoding.WriteObject(conn, modules.RPCSettings)
			if err != nil {
				return err
			}
			// COMPATv0.4.8 - If first decoding attempt fails, try decoding
			// into the old HostSettings type. Because we decode twice, we
			// must read the data into memory first.
			settingsBytes, err := encoding.ReadPrefix(conn, maxSettingsLen)
			if err != nil {
				return err
			}
			err = encoding.Unmarshal(settingsBytes, &settings)
			if err != nil {
				var oldSettings oldHostSettings
				err = encoding.Unmarshal(settingsBytes, &oldSettings)
				if err != nil {
					return err
				}
				// Convert the old type.
				settings = modules.HostSettings{
					NetAddress:   oldSettings.NetAddress,
					TotalStorage: oldSettings.TotalStorage,
					MinDuration:  oldSettings.MinDuration,
					MaxDuration:  oldSettings.MaxDuration,
					WindowSize:   oldSettings.WindowSize,
					Price:        oldSettings.Price,
					Collateral:   oldSettings.Collateral,
					UnlockHash:   oldSettings.UnlockHash,
				}
			}
			return nil
		}()

		// Now that network communication is done, lock the hostdb to modify the
		// host entry.
		func() {
			hdb.mu.Lock()
			defer hdb.mu.Unlock()

			// Regardless of whether the host responded, add it to allHosts.
			if _, exists := hdb.allHosts[hostEntry.NetAddress]; !exists {
				hdb.allHosts[hostEntry.NetAddress] = hostEntry
			}

			// If the scan was unsuccessful, decrement the host's reliability.
			if err != nil {
				hdb.decrementReliability(hostEntry.NetAddress, UnreachablePenalty)
				return
			}

			// Update the host settings, reliability, and weight. The old NetAddress
			// must be preserved.
			settings.NetAddress = hostEntry.HostSettings.NetAddress
			hostEntry.HostSettings = settings
			hostEntry.reliability = MaxReliability
			hostEntry.weight = calculateHostWeight(*hostEntry)
			hostEntry.online = true

			// If 'MaxActiveHosts' has not been reached, add the host to the
			// activeHosts tree.
			if _, exists := hdb.activeHosts[hostEntry.NetAddress]; !exists && len(hdb.activeHosts) < MaxActiveHosts {
				hdb.insertNode(hostEntry)
			}
		}()
	}
}

// threadedScan is an ongoing function which will query the full set of hosts
// every few hours to see who is online and available for uploading.
func (hdb *HostDB) threadedScan() {
	for {
		// Determine who to scan. At most 'MaxActiveHosts' will be scanned,
		// starting with the active hosts followed by a random selection of the
		// inactive hosts.
		func() {
			hdb.mu.Lock()
			defer hdb.mu.Unlock()

			// Scan all active hosts.
			for _, host := range hdb.activeHosts {
				hdb.scanHostEntry(host.hostEntry)
			}

			// Assemble all of the inactive hosts into a single array.
			var entries []*hostEntry
			for _, entry := range hdb.allHosts {
				entry2, exists := hdb.activeHosts[entry.NetAddress]
				if !exists {
					entries = append(entries, entry)
				} else {
					if build.DEBUG {
						if entry2.hostEntry != entry {
							panic("allHosts + activeHosts mismatch!")
						}
					}
				}
			}

			// Generate a random ordering of up to InactiveHostCheckupQuantity
			// hosts.
			n := InactiveHostCheckupQuantity
			if n > len(entries) {
				n = len(entries)
			}
			hostOrder, err := crypto.Perm(n)
			if err != nil {
				hdb.log.Println("ERR: could not generate random permutation:", err)
			}

			// Scan each host.
			for _, randIndex := range hostOrder {
				hdb.scanHostEntry(entries[randIndex])
			}
		}()

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
			}
			// If there's an error, sleep for the default amount of time.
			defaultBig := big.NewInt(int64(DefaultScanSleep))
			randSleep = defaultBig.Sub(defaultBig, minBig)
		}
		hdb.sleeper.Sleep(time.Duration(randSleep.Int64()) + MinScanSleep) // this means the MaxScanSleep is actual Max+Min.
	}
}
