package contractmanager

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
)

// ASF isa
//
// TODO: Need to vet that the maximum number of allowed storage folders has not
// been eclipsed.
func (wal *writeAheadLog) managedAddStorageFolder(sf storageFolder) error {
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

// cleanupUnfinishedStorageFolderAdditions will
func (wal *writeAheadLog) cleanupUnfinishedStorageFolderAdditions() error {
	for _, uc := range wal.uncommittedChanges {
		for _, usfa := range uc.UnfinishedStorageFolderAdditions {
			// The storage folder addition was interrupted due to an unexpected
			// error, and the change should be aborted. This can be completed
			// by simply removing the file that was partially created to house
			// the sectors that would have appeared in the storage folder.
			sectorHousingName := filepath.Join(usfa.Path, sectorFile)
			err := os.Remove(sectorHousingName)
			if err != nil {
				wal.cm.log.Println("Unable to remove documented sector housing:", sectorHousingName, err)
			}

			err = wal.appendChange(stateChange{
				ErroredStorageFolderAdditions: []string{usfa.Path},
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// commitAddStorageFolder integrates a pending AddStorageFolder call into the
// state. commitAddStorageFolder should only be called when finalizing an ACID
// transaction, and only after the WAL has been synced to disk, to ensure that
// the state change has been guaranteed even in the event of sudden power loss.
func (wal *writeAheadLog) commitAddStorageFolder(sf storageFolder) {
	wal.cm.storageFolders = append(wal.cm.storageFolders, &sf)
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

// AddStorageFolder adds a storage folder to the contract manager.
func (cm *ContractManager) AddStorageFolder(path string, size uint64) error {
	err := cm.tg.Add()
	if err != nil {
		return err
	}
	defer cm.tg.Done()

	// TODO: Document somewhere that we don't actually want to lock the
	// contract manager while we are adding the storage folder. All state
	// changes and state handling should be done in the WAL, which will do the
	// contract manager locking manually.

	// Check that the storage folder being added meets the size requirements.
	sectors := size / modules.SectorSize
	if sectors > maximumSectorsPerStorageFolder {
		// TODO: This should be consistent - min currently is size, and max is
		// sector count. Pick one.
		return errLargeStorageFolder
	}
	if size < minimumStorageFolderSize {
		return errSmallStorageFolder
	}
	if (size/modules.SectorSize)%storageFolderGranularity != 0 {
		return errStorageFolderGranularity
	}
	// Check that the path is an absolute path.
	if !filepath.IsAbs(path) {
		return errRelativePath
	}

	// Check that the folder being linked to both exists and is a folder.
	pathInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !pathInfo.Mode().IsDir() {
		return errStorageFolderNotFolder
	}

	// Create a storage folder object and add it to the WAL.
	newSF := storageFolder{
		Path:  path,
		Usage: make([]byte, size/modules.SectorSize/8),
	}
	return cm.wal.managedAddStorageFolder(newSF)
}
