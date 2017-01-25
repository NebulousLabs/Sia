package hostdb

// scan.go contains the functions which periodically scan the list of all hosts
// to see which hosts are online or offline, and to get any updates to the
// settings of the hosts.

import (
	"crypto/rand"
	"math/big"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	defaultScanSleep = 1*time.Hour + 37*time.Minute
	maxScanSleep     = 4 * time.Hour
	minScanSleep     = 1*time.Hour + 20*time.Minute

	hostCheckupQuantity = 250

	maxSettingsLen = 4e3

	hostRequestTimeout = 60 * time.Second
	hostScanDeadline   = 60 * time.Second

	// scanningThreads is the number of threads that will be probing hosts for
	// their settings and checking for reliability.
	scanningThreads = 20
)

// queueScan will add a host to the queue to be scanned.
func (hdb *HostDB) queueScan(entry modules.HostDBEntry) {
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

// managedUpdateEntry updates an entry in the hostdb after a scan has taken
// place.
//
// TODO: The operation of this function is highly dependent on the structure of
// the host weight function, changing the host weight function necessetates
// tuning of this function. This may stab us in the back.
func (hdb *HostDB) managedUpdateEntry(entry modules.HostDBEntry, newSettings modules.HostExternalSettings, netErr error) {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()

	// Grab the host from the host tree.
	newEntry, exists := hdb.hostTree.Select(entry.PublicKey)
	if !exists {
		newEntry = entry
	}

	// Add the datapoints for the scan.
	if len(newEntry.ScanHistory) < 2 {
		// Add two scans to the scan history. Two are needed because the scans
		// are forward looking, but we want this first scan to represent as
		// much as one week of uptime or downtime.
		earliestStartTime := time.Now().Add(time.Hour * 7 * 24 * -1)
		suggestedStartTime := time.Now().Add(time.Minute * 10 * time.Duration(hdb.blockHeight-entry.FirstSeen) * -1)
		if suggestedStartTime.Before(earliestStartTime) {
			suggestedStartTime = earliestStartTime
		}
		newEntry.ScanHistory = modules.HostDBScans{
			{Timestamp: suggestedStartTime, Success: netErr == nil},
			{Timestamp: time.Now(), Success: netErr == nil},
		}
	} else {
		newEntry.ScanHistory = append(newEntry.ScanHistory, modules.HostDBScan{Timestamp: time.Now(), Success: netErr == nil})
	}

	// Add the updated entry
	if !exists {
		err := hdb.hostTree.Insert(newEntry)
		if err != nil {
			hdb.log.Println("ERROR: unable to insert entry which is was thought to be new:", err)
		}
	} else {
		err := hdb.hostTree.Modify(newEntry)
		if err != nil {
			hdb.log.Println("ERROR: unable to modify entry which is thought to exist:", err)
		}
	}

	hdb.save()
}

// managedScanHost will connect to a host and grab the settings, verifying
// uptime and updating to the host's preferences.
func (hdb *HostDB) managedScanHost(entry modules.HostDBEntry) {
	// Request settings from the queued host entry.
	netAddr := entry.NetAddress
	pubKey := entry.PublicKey
	hdb.log.Debugf("Scanning host at %v: key %v", netAddr, pubKey)

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
		hdb.log.Debugf("Scan of host at %v failed: %v", netAddr, err)
	} else {
		hdb.log.Debugf("Scan of host at %v succeeded.", netAddr)
	}
	// Update the host tree to have a new entry, including the new error.
	hdb.managedUpdateEntry(entry, settings, err)
}

// threadedProbeHosts pulls hosts from the thread pool and runs a scan on them.
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

			// Introduce a scan for the most valuable hosts.
			checkups := hdb.hostTree.SelectRandom(hostCheckupQuantity, nil)
			hdb.log.Println("Performing scan on", len(checkups), "hosts")
			for _, host := range checkups {
				hdb.queueScan(host)
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
