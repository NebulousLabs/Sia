package contractmanager

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NebulousLabs/Sia/build"
)

// syncResources will call Sync on all resources that the WAL has open. The
// storage folder files will be left open, as they are not updated atomically.
// The settings file and WAL tmp files will be synced and closed, to perform an
// atomic update to the files.
func (wal *writeAheadLog) syncResources() {
	// Syncing occurs over multiple files and disks, and is done in parallel to
	// minimize the amount of time that a lock is held over the contract
	// manager.
	var wg sync.WaitGroup

	// Sync the settings file.
	wg.Add(1)
	go func() {
		defer wg.Done()

		tmpFilename := filepath.Join(wal.cm.persistDir, settingsFileTmp)
		filename := filepath.Join(wal.cm.persistDir, settingsFile)
		err := wal.fileSettingsTmp.Sync()
		if err != nil {
			wal.cm.log.Severe("ERROR: unable to sync the contract manager settings:", err)
		}
		err = wal.fileSettingsTmp.Close()
		if err != nil {
			wal.cm.log.Println("unable to close the temporary contract manager settings file:", err)
		}

		// For testing, provide a place to interrupt the saving of the sync
		// file. This makes it easy to simulate certain types of unclean
		// shutdown.
		if wal.cm.dependencies.disrupt("settingsSyncRename") {
			// The current settings file that is being re-written will not be
			// saved.
			return
		}

		err = wal.cm.dependencies.renameFile(tmpFilename, filename)
		if err != nil {
			wal.cm.log.Severe("ERROR: unable to atomically copy the contract manager settings:", err)
		}
	}()

	// Sync all of the storage folders.
	for _, sf := range wal.cm.storageFolders {
		// Skip operation on unavailable storage folders.
		if atomic.LoadUint64(&sf.atomicUnavailable) == 1 {
			continue
		}

		wg.Add(2)
		go func(sf *storageFolder) {
			defer wg.Done()
			err := sf.metadataFile.Sync()
			if err != nil {
				wal.cm.log.Severe("ERROR: unable to sync a storage folder:", err)
			}
		}(sf)
		go func(sf *storageFolder) {
			defer wg.Done()
			err := sf.sectorFile.Sync()
			if err != nil {
				wal.cm.log.Severe("ERROR: unable to sync a storage folder:", err)
			}
		}(sf)
	}

	// Sync the temp WAL file, but do not perform the atmoic rename - the
	// atomic rename must be guaranteed to happen after all of the other files
	// have been synced.
	wg.Add(1)
	go func() {
		defer wg.Done()

		err := wal.fileWal.Sync()
		if err != nil {
			wal.cm.log.Severe("Unable to sync the write-ahead-log:", err)
		}
	}()

	// Wait for all of the sync calls to finish.
	wg.Wait()

	// Now that the WAL is sync'd and updated, any calls waiting on ACID
	// guarantees can safely return.
	close(wal.syncChan)
	wal.syncChan = make(chan struct{})
}

// commit will take all of the changes that have been added to the WAL and
// atomically commit the WAL to disk, then apply the actions in the WAL to the
// state. commit will do lots of syncing disk I/O, and so can take a while,
// especially if there are a large number of actions queued up.
//
// A bool is returned indicating whether or not the commit was successful.
// False does not indiciate an error, it can also indicate that there was
// nothing to do.
//
// commit should only be called from threadedSyncLoop.
func (wal *writeAheadLog) commit() {
	// Sync all open, non-WAL files on the host.
	wal.syncResources()

	// Begin writing to the settings file.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Begin writing to the settings file, which will be synced during the
		// next iteration of the sync loop.
		var err error
		wal.fileSettingsTmp, err = wal.cm.dependencies.createFile(filepath.Join(wal.cm.persistDir, settingsFileTmp))
		if err != nil {
			wal.cm.log.Severe("Unable to open temporary settings file for writing:", err)
		}
		ss := wal.cm.savedSettings()
		b, err := json.MarshalIndent(ss, "", "\t")
		if err != nil {
			build.ExtendErr("unable to marshal settings data", err)
		}
		enc := json.NewEncoder(wal.fileSettingsTmp)
		if err := enc.Encode(settingsMetadata.Header); err != nil {
			build.ExtendErr("unable to write header to settings temp file", err)
		}
		if err := enc.Encode(settingsMetadata.Version); err != nil {
			build.ExtendErr("unable to write version to settings temp file", err)
		}
		if _, err = wal.fileSettingsTmp.Write(b); err != nil {
			build.ExtendErr("unable to write data settings temp file", err)
		}
	}()

	wg.Wait()
}

// spawnSyncLoop prepares and establishes the loop which will be running in the
// background to coordinate disk syncronizations. Disk syncing is done in a
// background loop to help with performance, and to allow multiple things to
// modify the WAL simultaneously.
func (wal *writeAheadLog) spawnSyncLoop() (err error) {
	// Create a signal so we know when the sync loop has stopped, which means
	// there will be no more open commits.
	threadsStopped := make(chan struct{})
	syncLoopStopped := make(chan struct{})
	wal.syncChan = make(chan struct{})
	go wal.threadedSyncLoop(threadsStopped, syncLoopStopped)
	wal.cm.tg.AfterStop(func() {
		// Wait for another iteration of the sync loop, so that the in-progress
		// settings can be saved atomically to disk.
		wal.mu.Lock()
		syncChan := wal.syncChan
		wal.mu.Unlock()
		<-syncChan

		// Close the threadsStopped channel to let the sync loop know that all
		// calls to tg.Add() in the contract manager have cleaned up.
		close(threadsStopped)

		// Because this is being called in an 'AfterStop' routine, all open
		// calls to the contract manager should have completed, and all open
		// threads should have closed. The last call to change the contract
		// manager should have completed, so the number of uncommitted changes
		// should be zero.
		<-syncLoopStopped // Wait for the sync loop to signal proper termination.

		// Allow unclean shutdown to be simulated by disrupting the removal of
		// the WAL file.
		err := wal.fileWal.Close()
		if err != nil {
			wal.cm.log.Println("ERROR: error closing wal file during contract manager shutdown:", err)
			return
		}

		if !wal.cm.dependencies.disrupt("cleanWALFile") {
			err = wal.cm.dependencies.removeFile(filepath.Join(wal.cm.persistDir, walFile))
			if err != nil {
				wal.cm.log.Println("Error removing WAL during contract manager shutdown:", err)
			}
		}
	})
	return nil
}

// resetWall checks the current size of the WAL and resets it if it approaches the max size.
func (wal *writeAheadLog) resetWAL() {
	// Only reset if no reset is in progress and the WAL is 80% full
	resetNeeded := !wal.resetInProgress && float64(wal.changeOffset) > 0.8*float64(maxWalSize)
	if !resetNeeded {
		return
	}

	// Start reset
	wal.resetInProgress = true
	go func() {
		wal.rmu.Lock()
		defer wal.rmu.Unlock()
		wal.mu.Lock()
		defer wal.mu.Unlock()

		wal.header.Revision += 1
		err := writeWALHeader(wal.fileWal, wal.header)
		if err != nil {
			panic(build.ExtendErr("Could not write WAL header during WAL reset."+
				"Crashing to prevent corruption.", err))
		}

		wal.changeOffset = headerLength() + wal.header.LengthMD

		wal.resetInProgress = false
	}()
}

// threadedSyncLoop is a background thread that occasionally commits the WAL to
// the state as an ACID transaction. This process can be very slow, so
// transactions to the contract manager are batched automatically and
// occasionally committed together.
func (wal *writeAheadLog) threadedSyncLoop(threadsStopped chan struct{}, syncLoopStopped chan struct{}) {
	// Provide a place for the testing to disable the sync loop.
	if wal.cm.dependencies.disrupt("threadedSyncLoopStart") {
		close(syncLoopStopped)
		return
	}

	syncInterval := 500 * time.Millisecond
	for {
		select {
		case <-threadsStopped:
			close(syncLoopStopped)
			return
		case <-time.After(syncInterval):
			// Commit all of the changes in the WAL to disk, and then apply the
			// changes.
			wal.mu.Lock()
			wal.commit()
			wal.resetWAL()
			wal.mu.Unlock()
		}
	}
}
