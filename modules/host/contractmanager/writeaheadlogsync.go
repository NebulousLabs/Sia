package contractmanager

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/persist"
)

// syncResources will call Sync on all resources that the WAL has open, except
// for the WAL temp file, which needs to be sync'd separately.
func (wal *writeAheadLog) syncResources() {
	// The syncing is done in parallel to ensure that the total amount of time
	// consumed here is approximately equivalent to the cost of Syncing the
	// slowest file, instead of waiting to sync some files until other files
	// have finished syncing.
	var wg sync.WaitGroup
	// Queue a thread to sync + copy-on-write the settings file.
	wg.Add(1)
	go func() {
		tmpFilename := filepath.Join(wal.cm.persistDir, settingsFileTmp)
		filename := filepath.Join(wal.cm.persistDir, settingsFile)
		// Sync the settings file.
		err := wal.fileSettingsTmp.Sync()
		if err != nil {
			wal.cm.log.Critical("ERROR: unable to sync the contract manager settings:", err)
			panic("unable to sync contract manager settings, crashing to avoid data corruption")
		}
		err = wal.fileSettingsTmp.Close()
		if err != nil {
			wal.cm.log.Println("unable to close the temporary contract manager settings file:", err)
		}
		// COW the settings file.
		err = os.Rename(tmpFilename, filename)
		if err != nil {
			wal.cm.log.Critical("ERROR: unable to atomically copy the contract manager settings:", err)
			panic("unable to atomically copy contract manager settings, crashing to avoid data corruption")
		}
		// Remove the temporary file as the contents are no longer relevant.
		// Another will be created shortly.
		err = os.Remove(tmpFilename)
		if err != nil {
			wal.cm.log.Println("ERROR: unable to remove temporary settings file:", err)
		}
		wg.Done()
	}()
	// Queue threads to sync all of the storage folders.
	for _, sf := range wal.cm.storageFolders {
		wg.Add(1)
		go func(sf *storageFolder) {
			err := sf.file.Sync()
			if err != nil {
				wal.cm.log.Critical("ERROR: unable to sync a storage folder:", err)
				panic("unable to sync a storage folder, creashing to avoid data corruption")
			}
			wg.Done()
		}(sf)
	}
	// Sync the temp WAL file, but DO NOT perform the copy-on-write - the COW
	// must happen AFTER all other files have successfully Sync'd.
	wg.Add(1)
	go func() {
		err := wal.fileWALTmp.Sync()
		if err != nil {
			wal.cm.log.Critical("Unable to sync the write-ahead-log:", err)
			panic("unable to sync the write-ahead-log, crashing to avoid data corrution")
		}
		err = wal.fileWALTmp.Close()
		if err != nil {
			// Log that the host is having trouble saving the uncommitted changes.
			// Crash if the list of uncommitted changes has grown very large.
			wal.cm.log.Println("ERROR: could not close temporary write-ahead-log in contract manager:", err)
		}
		wg.Done()
	}()
	wg.Wait()
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
	//
	// On the previous commit, a bunch of files were given operations to
	// perform, but the operations were not synced. They have now had a second
	// or so to do buffer + write, which means a Sync call here should take
	// significantly less time than it would have if Sync had been called right
	// away. This is important to performance, as Sync must be called under
	// lock.
	wal.syncResources()

	// The WAL file writes to a temporary file, so that there is a guarantee
	// that the actual WAL file will not ever be corrupted. Now that the tmp
	// file is closed and synced, rename the tmp file. Renaming is an atomic
	// action, meaning that the replacement to the WAL file is guaranteed not
	// to be corrupt.
	//
	// Note that this rename MUST occur AFTER all of the Sync calls above have
	// returned.
	walTmpName := filepath.Join(wal.cm.persistDir, walFileTmp)
	walFileName := filepath.Join(wal.cm.persistDir, walFile)
	err := os.Rename(walTmpName, walFileName)
	if err != nil {
		// Log that the host is having trouble saving the uncommitted changes.
		// Crash if the list of uncommitted changes has grown very large.
		wal.cm.log.Critical("ERROR: could not rename temporary write-ahead-log in contract manager:", err)
		panic("unable to copy-on-write the WAL temporary file, crashing to prevent data corruption")
	}
	// Remove the temporary WAL file as it has been committed. A new temporary
	// log file will be created shortly.
	err = os.Remove(walTmpName)
	if err != nil {
		wal.cm.log.Println("ERROR: unable to remove temporary write-ahead-log after successful commit-to-log:", err)
	}
	// Any module waiting for a synchronization has now had the changes
	// permanently committed, as of the WAL COW operation. Signal that syncing
	// is complete.
	close(wal.syncChan)
	wal.syncChan = make(chan struct{})

	// Take all of the changes that have been queued in the WAL and apply them
	// to the state. This will cause a lot of writing to the open files. These
	// writes will be Sync'd at the start of the next call to commit().
	for _, uc := range wal.uncommittedChanges {
		wal.applyChange(uc)
	}

	// Use helper functions to extract all remaining unfinished long-running
	// jobs from the WAL. Save these for later, when the WAL tmp file gets
	// reopened (the remaining unfinished jobs need to be written to the tmp
	// file to preserve their presence as understood by the WAL state).
	unfinishedAdditions := findUnfinishedStorageFolderAdditions(wal.uncommittedChanges)
	// Set the list of uncommitted changes to nil, as everything has been
	// applied and/or saved to be appended later.
	wal.uncommittedChanges = nil

	// In parallel, open up the WAL and write to it, and open up the settings
	// file and write to it.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		// Begin writing to the settings file, but do not Sync. Syncing here
		// can cause the function to block for a long time while holding the
		// WAL lock. Instead, sync on the next iteration when this function has
		// had over a second to commit the data to disk; the sync should be
		// much faster.
		var err error
		wal.fileSettingsTmp, err = os.Create(filepath.Join(wal.cm.persistDir, settingsFileTmp))
		if err != nil {
			wal.cm.log.Critical("Unable to open temporary settings file for writing:", err)
			panic("unable to open temporary settings file for writing, crashing to prevent data corruption")
		}
		ss := wal.cm.savedSettings()
		err = persist.Save(settingsMetadata, ss, wal.fileSettingsTmp)
		if err != nil {
			wal.cm.log.Critical("writing to settings tmp file has failed:", err)
			panic("unable to write to temporary settings file, crashing to avoid data corruption")
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		// Recreate the wal file so that it can receive new updates.
		wal.fileWALTmp, err = os.Create(walTmpName)
		if err != nil {
			wal.cm.log.Critical("ERROR: unable to create write-ahead-log:", err)
			panic("unable to create write-ahead-log in contract manager, crashing to avoid data loss")
		}
		// Write the metadata into the WAL.
		err := writeWALMetadata(wal.fileWALTmp)
		if err != nil {
			wal.cm.log.Critical("Unable to properly initialize WAL file, crashing to prevent corruption:", err)
			panic("Unable to properly initialize WAL file, crashing to prevent corruption.")
		}
		// Append all of the remaining long running uncommitted changes to the WAL.
		err = wal.appendChange(stateChange{
			UnfinishedStorageFolderAdditions: unfinishedAdditions,
		})
		if err != nil {
			wal.cm.log.Println("ERROR: in-progress action lost due to disk failure:", err)
		}
		wg.Done()
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
		// Close the threadsStopped channel to let the sync loop know that all
		// calls to tg.Add() in the contract manager have cleaned up.
		close(threadsStopped)

		// Because this is being called in an 'AfterStop' routine, all open
		// calls to the contract manager should have completed, and all open
		// threads should have closed. The last call to change the contract
		// manager should have completed, so the number of uncommitted changes
		// should be zero.
		<-syncLoopStopped // Wait for the sync loop to signal proper termination.

		// Perform a final sync to make sure all open ended changes get to
		// disk. This call is necessary because open-ended resources are not
		// sync'd during a commit - instead their sync is delayed until the
		// next call to commit() to maximize disk throughput and minimzie lock
		// contention.
		wal.syncResources()

		// Close the dangling commit resources.
		err = wal.fileWALTmp.Close()
		if err != nil {
			wal.cm.log.Println("Error closing wal file during contract manager shutdown:", err)
		}
		err = os.Remove(filepath.Join(wal.cm.persistDir, walFileTmp))
		if err != nil {
			wal.cm.log.Println("Error removing temporary WAL during contract manager shutdown:", err)
		}
		// Allow the removal of the WAL file to be disrupted to enable easier
		// simluations of power failure.
		if !wal.cm.dependencies.disrupt("cleanWALFile") {
			err = os.Remove(filepath.Join(wal.cm.persistDir, walFile))
			if err != nil {
				wal.cm.log.Println("Error removing WAL during contract manager shutdown:", err)
			}
		}
		for _, sf := range wal.cm.storageFolders {
			err = sf.file.Close()
			if err != nil {
				wal.cm.log.Println("Error closing the storage folder file handle", err)
			}
		}
	})
	return nil
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

	syncInterval := 1500 * time.Millisecond
	for {
		select {
		case <-threadsStopped:
			close(syncLoopStopped)
			return
		case <-time.After(syncInterval):
			// Commit all of the changes in the WAL to disk, and then apply the
			// changes. Measure how long it takes to apply the changes, and use
			// that to steer the amount of time that should be waited to the
			// next sync.
			wal.mu.Lock()
			wal.commit()
			wal.mu.Unlock()
		}
	}
}
