package hostdb

// scan.go contains the functions which periodically scan the list of all hosts
// to see which hosts are online or offline, and to get any updates to the
// settings of the hosts.

import (
	"net"
	"sort"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/fastrand"
)

// queueScan will add a host to the queue to be scanned. The host will be added
// at a random position which means that the order in which queueScan is called
// is not necessarily the order in which the hosts get scanned. That guarantees
// a random scan order during the initial scan.
func (hdb *HostDB) queueScan(entry modules.HostDBEntry) {
	// If this entry is already in the scan pool, can return immediately.
	_, exists := hdb.scanMap[entry.PublicKey.String()]
	if exists {
		return
	}
	// Add the entry to a random position in the waitlist.
	hdb.scanMap[entry.PublicKey.String()] = struct{}{}
	hdb.scanList = append(hdb.scanList, entry)
	if len(hdb.scanList) > 1 {
		i := len(hdb.scanList) - 1
		j := fastrand.Intn(i)
		hdb.scanList[i], hdb.scanList[j] = hdb.scanList[j], hdb.scanList[i]
	}
	// Check if any thread is currently emptying the waitlist. If not, spawn a
	// thread to empty the waitlist.
	if hdb.scanWait {
		// Another thread is emptying the scan list, nothing to worry about.
		return
	}

	// Sanity check - the scan map and the scan list should have the same
	// length.
	if build.DEBUG && len(hdb.scanMap) > len(hdb.scanList)+maxScanningThreads {
		hdb.log.Critical("The hostdb scan map has seemingly grown too large:", len(hdb.scanMap), len(hdb.scanList), maxScanningThreads)
	}

	hdb.scanWait = true
	go func() {
		scanPool := make(chan modules.HostDBEntry)
		defer close(scanPool)

		// Nobody is emptying the scan list, volunteer.
		if hdb.tg.Add() != nil {
			// Hostdb is shutting down, don't spin up another thread.  It is
			// okay to leave scanWait set to true as that will not affect
			// shutdown.
			return
		}
		defer hdb.tg.Done()

		// Block scan when a specific dependency is provided.
		hdb.deps.Disrupt("BlockScan")

		// Due to the patterns used to spin up scanning threads, it's possible
		// that we get to this point while all scanning threads are currently
		// used up, completing jobs that were sent out by the previous pool
		// managing thread. This thread is at risk of deadlocking if there's
		// not at least one scanning thread accepting work that it created
		// itself, so we use a starterThread exception and spin up
		// one-thread-too-many on the first iteration to ensure that we do not
		// deadlock.
		starterThread := false
		for {
			// If the scanList is empty, this thread can spin down.
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
			delete(hdb.scanMap, entry.PublicKey.String())
			scansRemaining := len(hdb.scanList)

			// Grab the most recent entry for this host.
			recentEntry, exists := hdb.hostTree.Select(entry.PublicKey)
			if exists {
				entry = recentEntry
			}

			// Try to send this entry to an existing idle worker (non-blocking).
			select {
			case scanPool <- entry:
				hdb.log.Debugf("Sending host %v for scan, %v hosts remain", entry.PublicKey.String(), scansRemaining)
				hdb.mu.Unlock()
				continue
			default:
			}

			// Create new worker thread.
			if hdb.scanningThreads < maxScanningThreads || !starterThread {
				starterThread = true
				hdb.scanningThreads++
				if err := hdb.tg.Add(); err != nil {
					hdb.mu.Unlock()
					return
				}
				go func() {
					defer hdb.tg.Done()
					hdb.threadedProbeHosts(scanPool)
					hdb.mu.Lock()
					hdb.scanningThreads--
					hdb.mu.Unlock()
				}()
			}
			hdb.mu.Unlock()

			// Block while waiting for an opening in the scan pool.
			hdb.log.Debugf("Sending host %v for scan, %v hosts remain", entry.PublicKey.String(), scansRemaining)
			select {
			case scanPool <- entry:
				// iterate again
			case <-hdb.tg.StopChan():
				// quit
				return
			}
		}
	}()
}

// updateEntry updates an entry in the hostdb after a scan has taken place.
//
// CAUTION: This function will automatically add multiple entries to a new host
// to give that host some base uptime. This makes this function co-dependent
// with the host weight functions. Adjustment of the host weight functions need
// to keep this function in mind, and vice-versa.
func (hdb *HostDB) updateEntry(entry modules.HostDBEntry, netErr error) {
	// If the scan failed because we don't have Internet access, toss out this update.
	if netErr != nil && !hdb.gateway.Online() {
		return
	}

	// Grab the host from the host tree, and update it with the neew settings.
	newEntry, exists := hdb.hostTree.Select(entry.PublicKey)
	if exists {
		newEntry.HostExternalSettings = entry.HostExternalSettings
	} else {
		newEntry = entry
	}

	// Update the recent interactions with this host.
	if netErr == nil {
		newEntry.RecentSuccessfulInteractions++
	} else {
		newEntry.RecentFailedInteractions++
	}

	// Add the datapoints for the scan.
	if len(newEntry.ScanHistory) < 2 {
		// Add two scans to the scan history. Two are needed because the scans
		// are forward looking, but we want this first scan to represent as
		// much as one week of uptime or downtime.
		earliestStartTime := time.Now().Add(time.Hour * 7 * 24 * -1)                                                   // Permit up to a week of starting uptime or downtime.
		suggestedStartTime := time.Now().Add(time.Minute * 10 * time.Duration(hdb.blockHeight-entry.FirstSeen+1) * -1) // Add one to the FirstSeen in case FirstSeen is this block, guarantees incrementing order.
		if suggestedStartTime.Before(earliestStartTime) {
			suggestedStartTime = earliestStartTime
		}
		newEntry.ScanHistory = modules.HostDBScans{
			{Timestamp: suggestedStartTime, Success: netErr == nil},
			{Timestamp: time.Now(), Success: netErr == nil},
		}
	} else {
		if newEntry.ScanHistory[len(newEntry.ScanHistory)-1].Success && netErr != nil {
			hdb.log.Debugf("Host %v is being downgraded from an online host to an offline host: %v\n", newEntry.PublicKey.String(), netErr)
		}

		// Make sure that the current time is after the timestamp of the
		// previous scan. It may not be if the system clock has changed. This
		// will prevent the sort-check sanity checks from triggering.
		newTimestamp := time.Now()
		prevTimestamp := newEntry.ScanHistory[len(newEntry.ScanHistory)-1].Timestamp
		if !newTimestamp.After(prevTimestamp) {
			newTimestamp = prevTimestamp.Add(time.Second)
		}

		// Before appending, make sure that the scan we just performed is
		// timestamped after the previous scan performed. It may not be if the
		// system clock has changed.
		newEntry.ScanHistory = append(newEntry.ScanHistory, modules.HostDBScan{Timestamp: newTimestamp, Success: netErr == nil})
	}

	// Check whether any of the recent scans demonstrate uptime. The pruning and
	// compression of the history ensure that there are only relatively recent
	// scans represented.
	var recentUptime bool
	for _, scan := range newEntry.ScanHistory {
		if scan.Success {
			recentUptime = true
		}
	}

	// If the host has been offline for too long, delete the host from the
	// hostdb. Only delete if there have been enough scans over a long enough
	// period to be confident that the host really is offline for good.
	if time.Now().Sub(newEntry.ScanHistory[0].Timestamp) > maxHostDowntime && !recentUptime && len(newEntry.ScanHistory) >= minScans {
		err := hdb.hostTree.Remove(newEntry.PublicKey)
		if err != nil {
			hdb.log.Println("ERROR: unable to remove host newEntry which has had a ton of downtime:", err)
		}

		// The function should terminate here as no more interaction is needed
		// with this host.
		return
	}

	// Compress any old scans into the historic values.
	for len(newEntry.ScanHistory) > minScans && time.Now().Sub(newEntry.ScanHistory[0].Timestamp) > maxHostDowntime {
		timePassed := newEntry.ScanHistory[1].Timestamp.Sub(newEntry.ScanHistory[0].Timestamp)
		if newEntry.ScanHistory[0].Success {
			newEntry.HistoricUptime += timePassed
		} else {
			newEntry.HistoricDowntime += timePassed
		}
		newEntry.ScanHistory = newEntry.ScanHistory[1:]
	}

	// Add the updated entry
	if !exists {
		err := hdb.hostTree.Insert(newEntry)
		if err != nil {
			hdb.log.Println("ERROR: unable to insert entry which is was thought to be new:", err)
		} else {
			hdb.log.Debugf("Adding host %v to the hostdb. Net error: %v\n", newEntry.PublicKey.String(), netErr)
		}
	} else {
		err := hdb.hostTree.Modify(newEntry)
		if err != nil {
			hdb.log.Println("ERROR: unable to modify entry which is thought to exist:", err)
		} else {
			hdb.log.Debugf("Adding host %v to the hostdb. Net error: %v\n", newEntry.PublicKey.String(), netErr)
		}
	}
}

// managedScanHost will connect to a host and grab the settings, verifying
// uptime and updating to the host's preferences.
func (hdb *HostDB) managedScanHost(entry modules.HostDBEntry) {
	// Request settings from the queued host entry.
	netAddr := entry.NetAddress
	pubKey := entry.PublicKey
	hdb.log.Debugf("Scanning host %v at %v", pubKey, netAddr)

	// Update historic interactions of entry if necessary
	hdb.mu.RLock()
	updateHostHistoricInteractions(&entry, hdb.blockHeight)
	hdb.mu.RUnlock()

	var settings modules.HostExternalSettings
	var latency time.Duration
	err := func() error {
		timeout := hostRequestTimeout
		hdb.mu.RLock()
		if len(hdb.initialScanLatencies) > minScansForSpeedup {
			build.Critical("initialScanLatencies should never be greater than minScansForSpeedup")
		}
		if !hdb.initialScanComplete && len(hdb.initialScanLatencies) == minScansForSpeedup {
			// During an initial scan, when we have at least minScansForSpeedup
			// active scans in initialScanLatencies, we use
			// 5*median(initialScanLatencies) as the new hostRequestTimeout to
			// speedup the scanning process.
			timeout = hdb.initialScanLatencies[len(hdb.initialScanLatencies)/2]
			timeout *= scanSpeedupMedianMultiplier
			if hostRequestTimeout < timeout {
				timeout = hostRequestTimeout
			}
		}
		hdb.mu.RUnlock()

		dialer := &net.Dialer{
			Cancel:  hdb.tg.StopChan(),
			Timeout: timeout,
		}
		start := time.Now()
		conn, err := dialer.Dial("tcp", string(netAddr))
		latency = time.Since(start)
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
		entry.HostExternalSettings = settings
	}
	success := err == nil

	hdb.mu.Lock()
	defer hdb.mu.Unlock()
	// Update the host tree to have a new entry, including the new error. Then
	// delete the entry from the scan map as the scan has been successful.
	hdb.updateEntry(entry, err)

	// Add the scan to the initialScanLatencies if it was successful.
	if success && len(hdb.initialScanLatencies) < minScansForSpeedup {
		hdb.initialScanLatencies = append(hdb.initialScanLatencies, latency)
		// If the slice has reached its maximum size we sort it.
		if len(hdb.initialScanLatencies) == minScansForSpeedup {
			sort.Slice(hdb.initialScanLatencies, func(i, j int) bool {
				return hdb.initialScanLatencies[i] < hdb.initialScanLatencies[j]
			})
		}
	}
}

// waitForScans is a helper function that blocks until the hostDB's scanList is
// empty.
func (hdb *HostDB) managedWaitForScans() {
	for {
		hdb.mu.Lock()
		length := len(hdb.scanList)
		hdb.mu.Unlock()
		if length == 0 {
			break
		}
		select {
		case <-hdb.tg.StopChan():
		case <-time.After(scanCheckInterval):
		}
	}
}

// threadedProbeHosts pulls hosts from the thread pool and runs a scan on them.
func (hdb *HostDB) threadedProbeHosts(scanPool <-chan modules.HostDBEntry) {
	for hostEntry := range scanPool {
		// Block until hostdb has internet connectivity.
		for {
			hdb.mu.RLock()
			online := hdb.gateway.Online()
			hdb.mu.RUnlock()
			if online {
				break
			}
			select {
			case <-time.After(time.Second * 30):
				continue
			case <-hdb.tg.StopChan():
				return
			}
		}

		// There appears to be internet connectivity, continue with the
		// scan.
		hdb.managedScanHost(hostEntry)
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

	// Wait until the consensus set is synced. Only then we can be sure that
	// the initial scan covers the whole network.
	for {
		if hdb.cs.Synced() {
			break
		}
		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(scanCheckInterval):
		}
	}

	// Block scan when a specific dependency is provided.
	hdb.deps.Disrupt("BlockScan")

	// The initial scan might have been interrupted. Queue one scan for every
	// announced host that was missed by the initial scan and wait for the
	// scans to finish before starting the scan loop.
	allHosts := hdb.hostTree.All()
	hdb.mu.Lock()
	for _, host := range allHosts {
		if len(host.ScanHistory) == 0 && host.HistoricUptime == 0 && host.HistoricDowntime == 0 {
			hdb.queueScan(host)
		}
	}
	hdb.mu.Unlock()
	hdb.managedWaitForScans()

	// Set the flag to indicate that the initial scan is complete.
	hdb.mu.Lock()
	hdb.initialScanComplete = true
	hdb.mu.Unlock()

	for {
		// Set up a scan for the hostCheckupQuanity most valuable hosts in the
		// hostdb. Hosts that fail their scans will be docked significantly,
		// pushing them further back in the hierarchy, ensuring that for the
		// most part only online hosts are getting scanned unless there are
		// fewer than hostCheckupQuantity of them.

		// Grab a set of hosts to scan, grab hosts that are active, inactive,
		// and offline to get high diversity.
		var onlineHosts, offlineHosts []modules.HostDBEntry
		allHosts := hdb.hostTree.All()
		for i := len(allHosts) - 1; i >= 0; i-- {
			if len(onlineHosts) >= hostCheckupQuantity && len(offlineHosts) >= hostCheckupQuantity {
				break
			}

			// Figure out if the host is online or offline.
			host := allHosts[i]
			online := len(host.ScanHistory) > 0 && host.ScanHistory[len(host.ScanHistory)-1].Success
			if online && len(onlineHosts) < hostCheckupQuantity {
				onlineHosts = append(onlineHosts, host)
			} else if !online && len(offlineHosts) < hostCheckupQuantity {
				offlineHosts = append(offlineHosts, host)
			}
		}

		// Queue the scans for each host.
		hdb.log.Println("Performing scan on", len(onlineHosts), "online hosts and", len(offlineHosts), "offline hosts.")
		hdb.mu.Lock()
		for _, host := range onlineHosts {
			hdb.queueScan(host)
		}
		for _, host := range offlineHosts {
			hdb.queueScan(host)
		}
		hdb.mu.Unlock()

		// Sleep for a random amount of time before doing another round of
		// scanning. The minimums and maximums keep the scan time reasonable,
		// while the randomness prevents the scanning from always happening at
		// the same time of day or week.
		sleepRange := uint64(maxScanSleep - minScanSleep)
		sleepTime := minScanSleep + time.Duration(fastrand.Uint64n(sleepRange))

		// Sleep until it's time for the next scan cycle.
		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(sleepTime):
		}
	}
}
