package contractmanager

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/persist"
)

type (
	// stateChange defines a change to the state that has not yet been applied
	// to the contract manager, but will be applied in a future commitment.
	stateChange struct {
		StorageFolderAdditions []storageFolder
		StorageFolderRemovals  []storageFolder
	}

	// In cases where nested locking is happening, the outside lock should be
	// held be the WAL, and not by the cm.
	writeAheadLog struct {
		file            *os.File
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
	err = wal.file.Write(changeBytes)
	if err != nil {
		return err
	}

	// Update the WAL to include the new storage folder in the uncommitted
	// changes.
	wal.uncommittedChanges = append(wal.uncommittedChanges, sc)
}

func (wal *writeAheadLog) commit() bool {
	if len(wal.uncommittedChanges) == 0 {
		return false
	}

	// Write the committed changes to disk.
	err = wal.file.Close()
	if err != nil {
		// Log that the host is having trouble saving the uncommitted changes.
		// Crash if the list of uncommitted changes has grown very large.
		h.log.Println("ERROR: could not save write-ahead-log in contract manager")
		if len(uncommittedChanges) > 250 {
			panic("persistent inability to save write-ahead-log, crashing")
		}
		return false
	}

	// Commit all of the changes to the state.
	for i := range wal.uncommittedChanges {
		for j := range wal.uncommittedChanges[i].storageFolderAdditions {
			wal.cm.storageFolders = append(wal.cm.storageFolders, wal.uncommittedChanges[i].storageFolderAdditions[j])
		}
		for j := range wal.uncommittedChanges[i].storageFolderRemovals {
			// Find the storge folder that's being removed and pull it from the contract manager.
			path := wal.uncommittedChanges[i].storageFolderRemovals[j]
			for k := range wal.cm.storageFolders {
				if wal.cm.storageFolders[k].Path == path {
					wal.cm.storageFolders = append(cm.storageFolders[0:k], cm.storageFolders[k+1:]...)
					break
				}
			}
		}
	}

	// Write all of the committed changes to disk.
	err = cm.save()
	if err != nil {
		// Log an error and abort the commitment. Commitments are not currently
		// reversible, which means that the only way to protect the contract
		// manager from corruption at this point is to crash. If we crash,
		// there will be no corruption as the save process is atomic.
		h.log.Critical("Could not save the contract manager - crashing to prevent corruption.")
		panic("Could not save the contract manager - crashing to prevent corruption.")
	}
	err = syscall.Sync()
	if err != nil {
		// Log an error and abort the commitment. Commitments are not currently
		// reversible, which means that the only way to protect the contract
		// manager from corruption at this point is to crash. If we crash,
		// there will be no corruption as the save process is atomic.
		h.log.Critical("Could not sync the contract manager - crashing to prevent corruption.")
		panic("Could not sync the contract manager - crashing to prevent corruption.")
	}
	// Recreate the wal file so that it can be appended to fresh.
	walFilename := filepath.Join(wal.cm.persistDir, walFile)
	wal.file, err = os.Create(walFilename)
	if err != nil {
		// Log the error, and crash the program. Without a working WAL, the
		// contract manager will not be able to service upload requests or
		// download requests, and is essentially useless. The user should be
		// able to recognize immediately that something significant has
		// happened.
		h.log.Println("ERROR: unable to create write-ahead-log")
		panic("unable to create write-ahead-log in contract manager")
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

// TODO: kill with tg.OnStop instead of <-tg.StopChan()
func (wal *writeAheadLog) threadedSyncLoop() {
	syncInterval := time.Millisecond * 100
	for {
		select {
		case <-wal.cm.tg.StopChan():
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
			// longer than 2 seconds.
			if synced {
				syncInterval = syncTime * 5
			}
			if syncInterval.Cmp(time.Second*2) > 0 {
				syncInterval = time.Second
			}
		}
	}
}

// ASF isa
func (wal *writeAheadLog) AddStorageFolder(sf *storageFolder) error {
	// AddStorageFolder needs to be able to read and write to the current set
	// of storage folders, which means a lock must be held on the storage
	// manager after confirming that the storage folder is safe.
	//
	// The WAL lock should be released at the end of the function, but only if
	// it was not released manually.
	manualUnlock := false
	wal.mu.Lock()
	defer func() {
		if !manualUnlock {
			wal.mu.Unlock()
		}
	}()

	// Check that the storage folder is not a duplicate. That requires first
	// checking the contract manager and then checking the WAL. An RLock is
	// held on the contract manager while viewing the contract manager data.
	// The safety of this function depends on the fact that the WAL will always
	// hold the outside lock in situations where both a WAL lock and a cm lock
	// are needed.
	duplicate := false
	cm.mu.RLock()
	for i := range cm.storageFolders {
		if cm.storageFolders[i].Path == sf.Path {
			duplicate = true
			break
		}
	}
	cm.mu.RUnlock()
	// Check the uncommitted changes for updates to the storage folders.
	for i := range wal.uncommittedChanges {
		for j := range wal.uncommittedChanges[i].additions {
			if wal.uncommittedChanges[i].additions[j].Path == sf.Path {
				duplicate = true
			}
		}
		for j := range wal.uncommittedChanges[i].removals {
			if wal.uncommittedChanges[i].removals[j].Path == sf.Path {
				duplicate = false
			}
		}
	}
	if duplicate {
		return errRepeatFolder
	}

	// Update the WAL to include the new storage folder in the uncommitted
	// changes.
	err = wal.appendChange(stateChange{
		additions: []storageFolderAddition{sf},
	})
	if err != nil {
		return err
	}

	// Unlock the WAL as we are done making modifications, but do not return
	// until the WAL has comitted to return the function.
	manualUnlock = true
	syncWait := wal.syncChan
	wal.mu.Unlock()
	<-syncWait
	return
}

// RSF is a
func (wal *writeAheadLog) RemoveStorageFolder(sf *storageFolder) error {
	// AddStorageFolder needs to be able to read and write to the current set
	// of storage folders, which means a lock must be held on the storage
	// manager after confirming that the storage folder is safe.
	//
	// The WAL lock should be released at the end of the function, but only if
	// it was not released manually.
	manualUnlock := false
	wal.mu.Lock()
	defer func() {
		if !manualUnlock {
			wal.mu.Unlock()
		}
	}()

	// Check that the storage folder exists. That requires first
	// checking the contract manager and then checking the WAL. An RLock is
	// held on the contract manager while viewing the contract manager data.
	// The safety of this function depends on the fact that the WAL will always
	// hold the outside lock in situations where both a WAL lock and a cm lock
	// are needed.
	exists := false
	cm.mu.RLock()
	for i := range cm.storageFolders {
		if cm.storageFolders[i].Path == sf.Path {
			exists = true
			break
		}
	}
	cm.mu.RUnlock()
	// Check the uncommitted changes for updates to the storage folders.
	for i := range wal.uncommittedChanges {
		for j := range wal.uncommittedChanges[i].additions {
			if wal.uncommittedChanges[i].additions[j].Path == sf.Path {
				exists = true
			}
		}
		for j := range wal.uncommittedChanges[i].removals {
			if wal.uncommittedChanges[i].removals[j].Path == sf.Path {
				exists = false
			}
		}
	}
	if !exists{
		return errStorageFolderNotFolder
	}

	// Update the WAL to include the new storage folder in the uncommitted
	// changes.
	err = wal.appendChange(stateChange{
		removals: []storageFolderRemoval{sf},
	})
	if err != nil {
		return err
	}

	// Unlock the WAL as we are done making modifications, but do not return
	// until the WAL has comitted to return the function.
	manualUnlock = true
	syncWait := wal.syncChan
	wal.mu.Unlock()
	<-syncWait
	return
}
