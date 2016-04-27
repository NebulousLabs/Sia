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

const (
	defaultScanSleep = 1*time.Hour + 37*time.Minute
	maxScanSleep     = 4 * time.Hour
	minScanSleep     = 1 * time.Hour

	maxActiveHosts              = 500
	inactiveHostCheckupQuantity = 250

	maxSettingsLen = 2e3

	hostRequestTimeout = 5 * time.Second

	// scanningThreads is the number of threads that will be probing hosts for
	// their settings and checking for reliability.
	scanningThreads = 25
)

// Reliability is a measure of a host's uptime.
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
		// TODO: should panic here
		return
	}
	entry.Reliability = entry.Reliability.Sub(penalty)
	entry.Online = false

	// If the entry is in the active database, remove it from the active
	// database.
	node, exists := hdb.activeHosts[addr]
	if exists {
		delete(hdb.activeHosts, entry.NetAddress)
		node.removeNode()
	}

	// If the reliability has fallen to 0, remove the host from the
	// database entirely.
	if entry.Reliability.IsZero() {
		delete(hdb.allHosts, addr)
	}
}

// threadedProbeHosts tries to fetch the settings of a host. If successful, the
// host is put in the set of active hosts. If unsuccessful, the host id deleted
// from the set of active hosts.
func (hdb *HostDB) threadedProbeHosts() {
	for hostEntry := range hdb.scanPool {
		// Request settings from the queued host entry.
		hdb.log.Debugln("Scanning", hostEntry.NetAddress)
		var settings modules.HostExternalSettings
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
			var pubkey crypto.PublicKey
			copy(pubkey[:], hostEntry.PublicKey.Key)
			return crypto.ReadSignedObject(conn, &settings, maxSettingsLen, pubkey)
		}()
		if err != nil {
			hdb.log.Debugln("Scanning", hostEntry.NetAddress, "failed", err)
		}

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
			settings.NetAddress = hostEntry.HostExternalSettings.NetAddress
			hostEntry.HostExternalSettings = settings
			hostEntry.Reliability = MaxReliability
			hostEntry.Weight = calculateHostWeight(*hostEntry)
			hostEntry.Online = true

			// If 'maxActiveHosts' has not been reached, add the host to the
			// activeHosts tree.
			if _, exists := hdb.activeHosts[hostEntry.NetAddress]; !exists && len(hdb.activeHosts) < maxActiveHosts {
				hdb.insertNode(hostEntry)
			}
			hdb.save()
		}()
	}
}

// threadedScan is an ongoing function which will query the full set of hosts
// every few hours to see who is online and available for uploading.
func (hdb *HostDB) threadedScan() {
	for {
		// Determine who to scan. At most 'maxActiveHosts' will be scanned,
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
				_, exists := hdb.activeHosts[entry.NetAddress]
				if !exists {
					entries = append(entries, entry)
				}
			}

			// Generate a random ordering of up to inactiveHostCheckupQuantity
			// hosts.
			hostOrder, err := crypto.Perm(len(entries))
			if err != nil {
				hdb.log.Println("ERR: could not generate random permutation:", err)
			}

			// Scan each host.
			for i := 0; i < len(hostOrder) && i < inactiveHostCheckupQuantity; i++ {
				hdb.scanHostEntry(entries[hostOrder[i]])
			}
		}()

		// Sleep for a random amount of time before doing another round of
		// scanning. The minimums and maximums keep the scan time reasonable,
		// while the randomness prevents the scanning from always happening at
		// the same time of day or week.
		maxBig := big.NewInt(int64(maxScanSleep))
		minBig := big.NewInt(int64(minScanSleep))
		randSleep, err := rand.Int(rand.Reader, maxBig.Sub(maxBig, minBig))
		if err != nil {
			build.Critical(err)
			// If there's an error, sleep for the default amount of time.
			defaultBig := big.NewInt(int64(defaultScanSleep))
			randSleep = defaultBig.Sub(defaultBig, minBig)
		}
		hdb.sleeper.Sleep(time.Duration(randSleep.Int64()) + minScanSleep)
	}
}
