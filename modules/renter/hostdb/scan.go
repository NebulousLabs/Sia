package hostdb

// scan.go contains the functions which periodically scan the list of all hosts
// to see which hosts are online or offline, and to get any updates to the
// settings of the hosts.

import (
	"bytes"
	"crypto/rand"
	"math/big"
	"net"
	"sort"
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
	minScanSleep     = 1*time.Hour + 20*time.Minute

	maxActiveHosts              = 500
	inactiveHostCheckupQuantity = 1000

	maxSettingsLen = 2e3

	hostRequestTimeout = 60 * time.Second
	hostScanDeadline   = 60 * time.Second

	// scanningThreads is the number of threads that will be probing hosts for
	// their settings and checking for reliability.
	scanningThreads = 50
)

// Reliability is a measure of a host's uptime.
var (
	MaxReliability     = types.NewCurrency64(500) // Given the scanning defaults, about 6 weeks of survival.
	DefaultReliability = types.NewCurrency64(150) // Given the scanning defaults, about 2 week of survival.
	UnreachablePenalty = types.NewCurrency64(1)
)

// queueHostEntry will add a host entry to the list of entries waiting to be
// scanned. If there is no thread that is currently walking through the scan
// list, one will be created and it will persist until shutdown or until the
// scan list is empty.
func (hdb *HostDB) queueHostEntry(entry *hostEntry) {
	// Add the entry to a waitlist, then check if any thread is currently
	// emptying the waitlist. If not, spawn a thread to empty the waitlist.
	hdb.scanList = append(hdb.scanList, entry)
	if hdb.scanWait {
		// Another thread is emptying the scan list, nothing to worry about.
		return
	}

	// Nobody is emptying the scan list, volunteer.
	if hdb.tg.Add() != nil {
		// Hostdb is shutting down, don't spin up another thread.
		return
	}
	hdb.scanWait = true
	go func() {
		defer hdb.tg.Done()

		for {
			hdb.mu.Lock()
			if len(hdb.scanList) == 0 {
				// Scan list is empty, can exit. Let the world know that nobody
				// is emptying the scan list anymore.
				hdb.scanWait = false
				hdb.mu.Unlock()
				return
			}
			// Get the next host, shrink the scan list.
			entry := hdb.scanList[0]
			hdb.scanList = hdb.scanList[1:]
			hdb.mu.Unlock()

			// Block while we wait for an opening in the scan pool.
			select {
			case hdb.scanPool <- entry:
				// iterate again
			case <-hdb.tg.StopChan():
				// quit
				return
			}
		}
	}()
}

// decrementReliability reduces the reliability of a node, moving it out of the
// set of active hosts or deleting it entirely if necessary.
func (hdb *HostDB) decrementReliability(addr modules.NetAddress, penalty types.Currency) {
	hdb.log.Debugln("reliability decrement issued for", addr)

	// Look up the entry and decrement the reliability.
	entry, exists := hdb.allHosts[addr]
	if !exists {
		build.Critical("host to be decremented did not exist in hostdb")
		return
	}
	entry.Reliability = entry.Reliability.Sub(penalty)

	// If the entry is in the active database, remove it from the active
	// database.
	existingEntry, exists := hdb.activeHosts[addr]
	if exists {
		hdb.log.Debugln("host is being pulled from list of active hosts", addr)
		hdb.hostTree.Remove(existingEntry.PublicKey)
		delete(hdb.activeHosts, entry.NetAddress)
	}

	// If the reliability has fallen to 0, remove the host from the
	// database entirely.
	if entry.Reliability.IsZero() {
		hdb.log.Debugln("host is being dropped from hostdb", addr)
		delete(hdb.allHosts, addr)
	}
}

// managedUpdateEntry updates an entry in the hostdb after a scan has taken
// place.
func (hdb *HostDB) managedUpdateEntry(entry *hostEntry, newSettings modules.HostExternalSettings, netErr error) {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()

	// Add a data point for the scan.
	entry.ScanHistory = append(entry.ScanHistory, modules.HostDBScan{
		Timestamp: time.Now(),
		Success:   netErr == nil,
	})
	// Ensure the scans are sorted.
	if !sort.IsSorted(entry.ScanHistory) {
		sort.Sort(entry.ScanHistory)
	}

	// Add the host to allHosts.
	priorHost, exists := hdb.allHosts[entry.NetAddress]
	if !exists {
		hdb.allHosts[entry.NetAddress] = entry
	}

	// If the scan was unsuccessful, decrement the host's reliability.
	if netErr != nil {
		if exists && bytes.Equal(priorHost.PublicKey.Key, entry.PublicKey.Key) {
			// Only decrement the reliability if the public key in the
			// hostdb matches the public key in the host announcement -
			// the failure may just be a failed signature, indicating
			// the wrong public key.
			hdb.decrementReliability(entry.NetAddress, UnreachablePenalty)
		}
		return
	}

	// The host entry should be updated to reflect the new weight. The safety
	// properties of the tree require that the weight does not change while the
	// node is in the tree, so the node must be removed before the settings and
	// weight are changed.
	existingEntry, exists := hdb.activeHosts[entry.NetAddress]
	if exists {
		hdb.hostTree.Remove(existingEntry.PublicKey)
		delete(hdb.activeHosts, entry.NetAddress)
	} else if len(hdb.activeHosts) > maxActiveHosts {
		// We already have the maximum number of active hosts, do not add more.
		return
	}

	// Update the host settings, reliability, and weight. The old NetAddress
	// must be preserved.
	newSettings.NetAddress = entry.HostExternalSettings.NetAddress
	entry.HostExternalSettings = newSettings
	entry.Reliability = MaxReliability
	err := hdb.hostTree.Insert(entry.HostDBEntry)
	hdb.activeHosts[entry.NetAddress] = entry
	if err != nil {
		// TODO: handle this error
	}

	// Sanity check - the node should be in the hostdb now.
	_, exists = hdb.activeHosts[entry.NetAddress]
	if !exists {
		hdb.log.Critical("Host was not added to the list of active hosts after the entry was updated.")
	}
	hdb.save()
}

// managedScanHost will connect to a host and grab the settings, verifying
// uptime and updating to the host's preferences.
func (hdb *HostDB) managedScanHost(hostEntry *hostEntry) {
	// Request settings from the queued host entry.
	//
	// A readlock is necessary when viewing the elements of the host entry.
	hdb.mu.RLock()
	netAddr := hostEntry.NetAddress
	pubKey := hostEntry.PublicKey
	hdb.mu.RUnlock()
	hdb.log.Debugln("Scanning", netAddr, pubKey)
	var settings modules.HostExternalSettings
	err := func() error {
		dialer := &net.Dialer{
			Cancel:  hdb.tg.StopChan(),
			Timeout: hostRequestTimeout,
		}
		conn, err := dialer.Dial("tcp", string(netAddr))
		if err != nil {
			return err
		}
		connCloseChan := make(chan struct{})
		go func() {
			select {
			case <-hdb.tg.StopChan():
			case <-connCloseChan:
			}
			conn.Close()
		}()
		defer close(connCloseChan)
		conn.SetDeadline(time.Now().Add(hostScanDeadline))

		err = encoding.WriteObject(conn, modules.RPCSettings)
		if err != nil {
			return err
		}
		var pubkey crypto.PublicKey
		copy(pubkey[:], pubKey.Key)
		return crypto.ReadSignedObject(conn, &settings, maxSettingsLen, pubkey)
	}()
	if err != nil {
		hdb.log.Debugln("Scanning", netAddr, pubKey, "failed:", err)
	} else {
		hdb.log.Debugln("Scanning", netAddr, pubKey, "succeeded")
	}

	// Update the host tree to have a new entry.
	hdb.managedUpdateEntry(hostEntry, settings, err)
}

// threadedProbeHosts tries to fetch the settings of a host. If successful, the
// host is put in the set of active hosts. If unsuccessful, the host id deleted
// from the set of active hosts.
func (hdb *HostDB) threadedProbeHosts() {
	err := hdb.tg.Add()
	if err != nil {
		return
	}
	defer hdb.tg.Done()

	for {
		select {
		case <-hdb.tg.StopChan():
			return
		case hostEntry := <-hdb.scanPool:
			hdb.managedScanHost(hostEntry)
		}
	}
}

// threadedScan is an ongoing function which will query the full set of hosts
// every few hours to see who is online and available for uploading.
func (hdb *HostDB) threadedScan() {
	err := hdb.tg.Add()
	if err != nil {
		return
	}
	defer hdb.tg.Done()

	for {
		// Determine who to scan. At most 'maxActiveHosts' will be scanned,
		// starting with the active hosts followed by a random selection of the
		// inactive hosts.
		func() {
			hdb.mu.Lock()
			defer hdb.mu.Unlock()

			// Scan all active hosts.
			for _, host := range hdb.activeHosts {
				hdb.queueHostEntry(host)
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
				hdb.queueHostEntry(entries[hostOrder[i]])
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

		// Sleep until it's time for the next scan cycle.
		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(time.Duration(randSleep.Int64()) + minScanSleep):
		}
	}
}
