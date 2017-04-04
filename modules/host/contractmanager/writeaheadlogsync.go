package contractmanager

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/persist"
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
			panic("unable to sync contract manager settings, crashing to avoid data corruption")
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
			panic("unable to atomically copy contract manager settings, crashing to avoid data corruption")
		}
	}()

	// Sync all of the storage folders.
	for _, sf := range wal.cm.storageFolders {
		wg.Add(2)
		go func(sf *storageFolder) {
			defer wg.Done()
			err := sf.metadataFile.Sync()
			if err != nil {
				wal.cm.log.Severe("ERROR: unable to sync a storage folder:", err)
				panic("unable to sync a storage folder, creashing to avoid data corruption")
			}
		}(sf)
		go func(sf *storageFolder) {
			defer wg.Done()
			err := sf.sectorFile.Sync()
			if err != nil {
				wal.cm.log.Severe("ERROR: unable to sync a storage folder:", err)
				panic("unable to sync a storage folder, creashing to avoid data corruption")
			}
		}(sf)
	}

	// Sync the temp WAL file, but do not perform the atmoic rename - the
	// atomic rename must be guaranteed to happen after all of the other files
	// have been synced.
	wg.Add(1)
	go func() {
		defer wg.Done()

		err := wal.fileWALTmp.Sync()
		if err != nil {
			wal.cm.log.Severe("Unable to sync the write-ahead-log:", err)
			panic("unable to sync the write-ahead-log, crashing to avoid data corrution")
		}
		err = wal.fileWALTmp.Close()
		if err != nil {
			// Log that the host is having trouble saving the uncommitted changes.
			// Crash if the list of uncommitted changes has grown very large.
			wal.cm.log.Println("ERROR: could not close temporary write-ahead-log in contract manager:", err)
			return
		}
	}()

	// Wait for all of the sync calls to finish.
	wg.Wait()

	// Now that all the Sync calls have completed, rename the WAL tmp file to
	// update the WAL.
	if !wal.cm.dependencies.disrupt("walRename") {
		walTmpName := filepath.Join(wal.cm.persistDir, walFileTmp)
		walFileName := filepath.Join(wal.cm.persistDir, walFile)
		err := wal.cm.dependencies.renameFile(walTmpName, walFileName)
		if err != nil {
			// Log that the host is having trouble saving the uncommitted changes.
			// Crash if the list of uncommitted changes has grown very large.
			wal.cm.log.Severe("ERROR: could not rename temporary write-ahead-log in contract manager:", err)
			panic("unable to copy-on-write the WAL temporary file, crashing to prevent data corruption")
		}
	}

	// Perform any cleanup actions on the updates.
	for _, sc := range wal.uncommittedChanges {
		for _, sfe := range sc.StorageFolderExtensions {
			wal.commitStorageFolderExtension(sfe)
		}
		for _, sfr := range sc.StorageFolderReductions {
			wal.commitStorageFolderReduction(sfr)
		}
		for _, sfr := range sc.StorageFolderRemovals {
			wal.commitStorageFolderRemoval(sfr)
		}

		// TODO: Virtual sector handling here.
	}

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

	// Extract any unfinished long-running jobs from the list of WAL items.
	unfinishedAdditions := findUnfinishedStorageFolderAdditions(wal.uncommittedChanges)
	unfinishedExtensions := findUnfinishedStorageFolderExtensions(wal.uncommittedChanges)

	// Clear the set of uncommitted changes.
	wal.uncommittedChanges = nil

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
			panic("unable to open temporary settings file for writing, crashing to prevent data corruption")
		}
		ss := wal.cm.savedSettings()
		err = persist.Save(settingsMetadata, ss, wal.fileSettingsTmp)
		if err != nil {
			wal.cm.log.Severe("writing to settings tmp file has failed:", err)
			panic("unable to write to temporary settings file, crashing to avoid data corruption")
		}
	}()

	// Begin writing new changes to the WAL.
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Recreate the wal file so that it can receive new updates.
		var err error
		walTmpName := filepath.Join(wal.cm.persistDir, walFileTmp)
		wal.fileWALTmp, err = wal.cm.dependencies.createFile(walTmpName)
		if err != nil {
			wal.cm.log.Severe("ERROR: unable to create write-ahead-log:", err)
			panic("unable to create write-ahead-log in contract manager, crashing to avoid data loss")
		}
		// Write the metadata into the WAL.
		err = writeWALMetadata(wal.fileWALTmp)
		if err != nil {
			wal.cm.log.Severe("Unable to properly initialize WAL file, crashing to prevent corruption:", err)
			panic("Unable to properly initialize WAL file, crashing to prevent corruption.")
		}

		// Append all of the remaining long running uncommitted changes to the WAL.
		wal.appendChange(stateChange{
			UnfinishedStorageFolderAdditions:  unfinishedAdditions,
			UnfinishedStorageFolderExtensions: unfinishedExtensions,
		})
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
		if !wal.cm.dependencies.disrupt("cleanWALFile") {
			err = wal.cm.dependencies.removeFile(filepath.Join(wal.cm.persistDir, walFile))
			if err != nil {
				wal.cm.log.Println("Error removing WAL during contract manager shutdown:", err)
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
			wal.mu.Unlock()
		}
	}
}
