package contractmanager

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type (
	// stateChange defines a change to the state that has not yet been applied
	// to the contract manager, but will be applied in a future commitment.
	stateChange struct {
		// Fields related to adding a storage folder. When a storage folder is
		// added, it first appears in the 'unfinished storage folder additions'
		// field. A lot of I/O is incurred when adding a storage folder, and a
		// WAL lock should not be held throughout the preparation operations.
		// When the I/O is complete, the WAL is locked again and the unfinished
		// storage folder is moved to the StorageFolderAdditions list.
		//
		// When committing, only StorageFolderAdditions will be committed to
		// the state. Some filtering is executed at commit time to determine
		// which UnfinishedStorageFolderAdditions need to be preserved through
		// to the next commit. Any unfinished storage folders which end up in
		// the StorageFolderAdditions list will not need to be preserved. If
		// there is an error during the I/O preperation, the storage folder
		// will be cleaned up and added to the ErroredStorageFolderAdditions
		// field, which also means that the storage folder does not need to be
		// preserved.
		ErroredStorageFolderAdditions    []string
		StorageFolderAdditions           []storageFolder
		UnfinishedStorageFolderAdditions []storageFolder
	}

	// In cases where nested locking is happening, the outside lock should be
	// held be the WAL, and not by the cm.
	writeAheadLog struct {
		file               *os.File
		syncChan           chan struct{}
		uncommittedChanges []stateChange

		// Utilities. The WAL needs access to the ContractManager because all
		// mutations to ACID fields of the contract manager happen through the
		// WAL.
		cm *ContractManager
		mu sync.Mutex
	}
)

func (wal *writeAheadLog) appendChange(sc stateChange) error {
	// Marshal the change and then write the change to the WAL file. Do not
	// sync the WAL file, as this operation does not need to guarantee that the
	// data hits the platter, the syncLoop will handle that piece.
	changeBytes, err := json.MarshalIndent(sc, "", "\t")
	if err != nil {
		return err
	}
	_, err = wal.file.Write(changeBytes)
	if err != nil {
		return err
	}

	// Update the WAL to include the new storage folder in the uncommitted
	// changes.
	wal.uncommittedChanges = append(wal.uncommittedChanges, sc)
	return nil
}

// commit
//
// commit should only be called from threadedSyncLoop.
func (wal *writeAheadLog) commit() bool {
	if len(wal.uncommittedChanges) == 0 {
		return false
	}

	// Write the committed changes to disk.
	err := wal.file.Sync()
	if err != nil {
		wal.cm.log.Critical("Could not call sync before migrating the host write-ahead-log")
		panic("could not sync to disk while writing the write-ahead-log in the host, corruption possible, crashing to prevent corruption")
	}
	err = wal.file.Close()
	if err != nil {
		// Log that the host is having trouble saving the uncommitted changes.
		// Crash if the list of uncommitted changes has grown very large.
		wal.cm.log.Println("ERROR: could not close temporary write-ahead-log in contract manager")
		if len(wal.uncommittedChanges) > 250 {
			panic("persistent inability to close write-ahead-log, crashing")
		}
		return false
	}
	walTmpName := filepath.Join(wal.cm.persistDir, walFileTmp)
	walFileName := filepath.Join(wal.cm.persistDir, walFile)
	err = os.Rename(walTmpName, walFileName)
	if err != nil {
		// Log that the host is having trouble saving the uncommitted changes.
		// Crash if the list of uncommitted changes has grown very large.
		wal.cm.log.Println("ERROR: could not rename temporary write-ahead-log in contract manager")
		if len(wal.uncommittedChanges) > 250 {
			panic("persistent inability to rename temporary write-ahead-log, crashing")
		}
		return false
	}
	err = os.Remove(walTmpName)
	if err != nil {
		wal.cm.log.Println("ERROR: unable to remove temporary write-ahead-log after successful commit-to-log")
	}

	// Commit all of the changes to the state.
	for _, uc := range wal.uncommittedChanges {
		for _, sfa := range uc.StorageFolderAdditions {
			wal.commitAddStorageFolder(sfa)
		}
	}
	// Save all of the unfinished long-running tasks.
	unfinishedAdditions := wal.findUnfinishedStorageFolderAdditions(wal.uncommittedChanges)
	// Done applying uncommitted changes, set them to 'nil'.
	wal.uncommittedChanges = nil

	// Write all of the committed changes to disk.
	err = wal.cm.saveSync()
	if err != nil {
		// Log an error and abort the commitment. Commitments are not currently
		// reversible, which means that the only way to protect the contract
		// manager from corruption at this point is to crash. If we crash,
		// there will be no corruption as the save process is atomic.
		wal.cm.log.Critical("Could not save the contract manager - crashing to prevent corruption.")
		panic("Could not save the contract manager - crashing to prevent corruption.")
	}
	// Recreate the wal file so that it can be appended to fresh.
	wal.file, err = os.Create(walTmpName)
	if err != nil {
		// Log the error, and crash the program. Without a working WAL, the
		// contract manager will not be able to service upload requests or
		// download requests, and is essentially useless. The user should be
		// able to recognize immediately that something significant has
		// happened.
		wal.cm.log.Println("ERROR: unable to create write-ahead-log")
		panic("unable to create write-ahead-log in contract manager")
	}
	// Write all of the long running uncommitted changes to the WAL
	// immediately.
	for _, change := range unfinishedAdditions {
		err = wal.appendChange(change)
		if err != nil {
			wal.cm.log.Println("ERROR: in-progress action lost due to disk failure")
		}
	}

	// The state has been updated successfully. Notify all functions that are
	// blocking while waiting for a commitment confirmation that the commit has
	// been successful by closing the syncChan.
	close(wal.syncChan)
	// Create another syncChan so that future changes to the log can safely
	// select on the close channel.
	wal.syncChan = make(chan struct{})
	// Let the caller know that the commit was successful.
	return true
}

// TODO: document all of the potentially long-running unfinished tasks and
// determine how they may affect where a sector gets placed.
func (wal *writeAheadLog) load() error {
	// Open up the WAL file.
	walFileName := filepath.Join(wal.cm.persistDir, walFile)
	file, err := os.Open(walFileName)
	if os.IsNotExist(err) {
		// There is no WAL, which most likely means that everything has
		// committed safely, and that there was a clean shutdown.
		return nil
	} else if err != nil {
		return err
	}
	defer file.Close()

	// Read changes from the WAL one at a time and load them back into memory.
	// Because they are already in the WAL, they do not need to be written to
	// the WAL again.
	var sc stateChange
	decoder := json.NewDecoder(file)
	for err == nil {
		err = decoder.Decode(&sc)
		if err == nil {
			wal.uncommittedChanges = append(wal.uncommittedChanges, sc)
		}
	}
	if err != io.EOF {
		return err
	}

	// Do any cleanup regarding long-running unfinished tasks.
	wal.cleanupUnfinishedStorageFolderAdditions()

	return nil
}

func (wal *writeAheadLog) spawnSyncLoop() (err error) {
	// Create the file resource that gets used when committing. Then establish
	// the AfterStop call that will close the file resource.
	walTmpName := filepath.Join(wal.cm.persistDir, walFileTmp)
	wal.file, err = os.Open(walTmpName)
	if err != nil {
		return err
	}

	// Create a signal so we know when the sync loop has stopped, which means
	// there will be no more open commits.
	syncLoopStopped := make(chan struct{})
	go wal.threadedSyncLoop(syncLoopStopped)
	wal.cm.tg.AfterStop(func() {
		// Because this is being called in an 'AfterStop' routine, all open
		// calls to the contract manager should have completed, and all open
		// threads should have closed. The last call to change the contract
		// manager should have completed, so the number of uncommitted changes
		// should be zero.
		<-syncLoopStopped // Wait for the sync loop to signal proper termination.
		if len(wal.uncommittedChanges) != 0 {
			wal.cm.log.Critical("Unsafe shutdown, contract manager has uncommitted changes yet the sync loop is terminating.")
		}

		// Close the dangling commit resources.
		err = wal.file.Close()
		if err != nil {
			wal.cm.log.Println("Error closing walfile during contract manager shutdown:", err)
		}
	})
	return nil
}

func (wal *writeAheadLog) threadedSyncLoop(syncLoopStopped chan struct{}) {
	syncInterval := time.Millisecond * 100
	for {
		select {
		case <-wal.cm.tg.StopChan():
			close(syncLoopStopped)
			return
		case <-time.After(syncInterval):
			// Commit all of the changes in the WAL to disk, and then apply the
			// changes. Measure how long it takes to apply the changes, and use
			// that to steer the amount of time that should be waited to the
			// next sync.
			start := time.Now()
			wal.mu.Lock()
			synced := wal.commit()
			wal.mu.Unlock()
			syncTime := time.Since(start)

			// Wait 5x as long as the previous sync took, but do not wait
			// longer than a second.
			if synced {
				syncInterval = syncTime * 5
			}
			if syncInterval > time.Second {
				syncInterval = time.Second
			}
		}
	}
}
