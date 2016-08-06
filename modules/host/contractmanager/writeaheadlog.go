package contractmanager

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

type (
	// stateChange defines a change to the state that has not yet been applied
	// to the contract manager, but will be applied in a future commitment.
	stateChange struct {
		StorageFolderAdditions []storageFolder
		StorageFolderRemovals  []storageFolder

		ErroredStorageFolderAdditions    []string
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

func (wal *writeAheadLog) findUnfinishedStorageFolderAdditions(scs []stateChange) []stateChange {
	// Create a map containing a list of all unfinished storage folder
	// additions identified by their path to the storage folder that is being
	// added.
	usfMap := make(map[string]storageFolder)
	for _, sc := range scs {
		for _, sf := range sc.UnfinishedStorageFolderAdditions {
			usfMap[sf.Path] = sf
		}
		for _, sf := range sc.StorageFolderAdditions {
			delete(usfMap, sf.Path)
		}
		for _, path := range sc.ErroredStorageFolderAdditions {
			delete(usfMap, path)
		}
	}

	var sfs []storageFolder
	for _, sf := range usfMap {
		sfs = append(sfs, sf)
	}
	return []stateChange{{
		UnfinishedStorageFolderAdditions: sfs,
	}}
}

// commitAddStorageFolder integrates a pending AddStorageFolder call into the
// state. commitAddStorageFolder should only be called when finalizing an ACID
// transaction, and only after the WAL has been synced to disk, to ensure that
// the state change has been guaranteed even in the event of sudden power loss.
func (wal *writeAheadLog) commitAddStorageFolder(sf storageFolder) {
	wal.cm.storageFolders = append(wal.cm.storageFolders, &sf)
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
	for i, uc := range wal.uncommittedChanges {
		for _, usfa := range uc.UnfinishedStorageFolderAdditions {
			// The storage folder addition was interrupted due to an unexpected
			// error, and the change should be aborted. This can be completed
			// by simply removing the file that was partially created to house
			// the sectors that would have appeared in the storage folder.
			sectorHousingName := filepath.Join(usfa.Path, sectorFile)
			os.Remove(sectorHousingName)
		}
		wal.uncommittedChanges[i].UnfinishedStorageFolderAdditions = nil
	}
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

// ASF isa
func (wal *writeAheadLog) AddStorageFolder(sf storageFolder) error {
	err := func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()
		// Check that the storage folder is not a duplicate. That requires first
		// checking the contract manager and then checking the WAL. An RLock is
		// held on the contract manager while viewing the contract manager data.
		// The safety of this function depends on the fact that the WAL will always
		// hold the outside lock in situations where both a WAL lock and a cm lock
		// are needed.
		duplicate := false
		wal.cm.mu.RLock()
		for i := range wal.cm.storageFolders {
			if wal.cm.storageFolders[i].Path == sf.Path {
				duplicate = true
				break
			}
		}
		wal.cm.mu.RUnlock()
		// Check the uncommitted changes for updates to the storage folders which
		// alter the 'duplicate' status of this storage folder.
		for i := range wal.uncommittedChanges {
			for j := range wal.uncommittedChanges[i].StorageFolderAdditions {
				if wal.uncommittedChanges[i].StorageFolderAdditions[j].Path == sf.Path {
					duplicate = true
				}
			}
			for j := range wal.uncommittedChanges[i].StorageFolderRemovals {
				if wal.uncommittedChanges[i].StorageFolderRemovals[j].Path == sf.Path {
					duplicate = false
				}
			}
		}
		for _, uc := range wal.uncommittedChanges {
			for _, usfa := range uc.UnfinishedStorageFolderAdditions {
				if usfa.Path == sf.Path {
					duplicate = true
				}
			}
		}
		if duplicate {
			return errRepeatFolder
		}

		// Add the storage folder to the list of unfinished storage folder
		// additions, so that no naming conflicts can appear while this storage
		// folder is being processed.
		wal.appendChange(stateChange{
			UnfinishedStorageFolderAdditions: []storageFolder{sf},
		})
		return nil
	}()
	if err != nil {
		return err
	}
	// If there's an error in the rest of the function, the storage folder
	// needs to be removed from the list of unfinished storage folder
	// additions.
	defer func() {
		if err != nil {
			wal.mu.Lock()
			defer wal.mu.Unlock()
			err = wal.appendChange(stateChange{
				ErroredStorageFolderAdditions: []string{sf.Path},
			})
		}
	}()

	// The WAL now contains a commitment to create the storage folder, but the
	// storage folder actually needs to be created. The storage folder should
	// be big enough to house all the potential sectors, and also needs to
	// contain a prefix that is 16 bytes per sector which indicates where each
	// sector is located on disk.
	numSectors := uint64(len(sf.Usage)) * 2
	sectorLookupSize := numSectors * 16
	sectorHousingSize := numSectors * modules.SectorSize
	// Open a file and write empty data across the whole file to reserve space
	// on disk for sector activities.
	sectorHousingName := filepath.Join(sf.Path, sectorFile)
	file, err := os.Open(sectorHousingName)
	if err != nil {
		return err
	}
	defer file.Close()
	defer func() {
		if err != nil {
			err := os.Remove(sectorHousingName)
			if err != nil {
				wal.cm.log.Println("could not remove sector housing after failed storage folder add:", err)
			}
		}
	}()
	totalSize := sectorLookupSize + sectorHousingSize
	hundredMBWrites := totalSize / 100e6
	finalWriteSize := totalSize % 100e6
	hundredMB := make([]byte, 100e6)
	finalBytes := make([]byte, finalWriteSize)
	for i := uint64(0); i < hundredMBWrites; i++ {
		_, err = file.Write(hundredMB)
		if err != nil {
			return err
		}
	}
	_, err = file.Write(finalBytes)
	if err != nil {
		return err
	}
	err = file.Sync()
	if err != nil {
		return err
	}

	// Update the WAL to include the new storage folder in the uncommitted
	// changes.
	wal.mu.Lock()
	err = wal.appendChange(stateChange{
		StorageFolderAdditions: []storageFolder{sf},
	})
	wal.mu.Unlock()
	if err != nil {
		return err
	}

	// Unlock the WAL as we are done making modifications, but do not return
	// until the WAL has comitted to return the function.
	syncWait := wal.syncChan
	<-syncWait
	return nil
}
