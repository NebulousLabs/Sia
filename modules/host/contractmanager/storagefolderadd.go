package contractmanager

import (
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

// managedAddStorageFolder is a WAL operation to add a storage folder to the
// contract manager. Some of the error checking has already been performed by
// the parent function, contractmanager.AddStorageFolder. The parent function
// performs checking that can be done without access to the state, and
// writeAheadLog.managedAddStorageFolder will perform all checking that
// requires access to the state.
//
// managedAddStorageFolder can take a long time, as it writes a giant, zeroed
// out file to disk covering the entire range of the storage folder. Having the
// WAL locked throughout the whole operation is unacceptable, which means that
// some additional management is needed to make sure that concurrent calls to
// managedAddStorageFolder do not conflict or race. The WAL is adapted to
// support long running operations which may eventually change the state, but
// which also may eventually fail - three fields are then used. One to indicate
// that process for adding a storage folder has begun, one to indicate that the
// process for adding a storage folder has completed, and one to indicate that
// the process for adding a storage folder has failed.
//
// TODO: We can use fallocate on linux systems to speed things up. If a Windows
// equivalent cannot be found, it is acceptable that Linux systems would have
// better performance.
func (wal *writeAheadLog) managedAddStorageFolder(sf *storageFolder) error {
	// Precompute the total on-disk size of the storage folder. The storage
	// folder needs to have modules.SectorSize available for each sector, plus
	// another 16 bytes per sector to store a mapping from sector id to its
	// location in the storage folder.
	numSectors := uint64(len(sf.Usage)) * 32
	sectorLookupSize := numSectors * 16
	sectorHousingSize := numSectors * modules.SectorSize
	totalSize := sectorLookupSize + sectorHousingSize

	// Update the uncommitted state to include the storage folder, returning an
	// error if any checks fail.
	err := func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		// Check that the storage folder is not a duplicate. That requires first
		// checking the contract manager and then checking the WAL. An RLock is
		// held on the contract manager while viewing the contract manager data.
		// The safety of this function depends on the fact that the WAL will always
		// hold the outside lock in situations where both a WAL lock and a cm lock
		// are needed.
		//
		// The number of storage folders are also counted, to make sure that
		// the maximum number of storage folders allowed is not exceeded.
		var err error
		wal.cm.mu.RLock()
		numStorageFolders := uint64(len(wal.cm.storageFolders))
		for i := range wal.cm.storageFolders {
			if wal.cm.storageFolders[i].Path == sf.Path {
				err = errRepeatFolder
				break
			}
		}
		wal.cm.mu.RUnlock()
		if err != nil {
			return err
		}

		// Check the uncommitted changes for updates to the storage folders
		// which alter the 'duplicate' status of this storage folder.
		for i := range wal.uncommittedChanges {
			for j := range wal.uncommittedChanges[i].StorageFolderAdditions {
				if wal.uncommittedChanges[i].StorageFolderAdditions[j].Path == sf.Path {
					return errRepeatFolder
				}
			}
		}
		for _, uc := range wal.uncommittedChanges {
			for _, usfa := range uc.UnfinishedStorageFolderAdditions {
				if usfa.Path == sf.Path {
					return errRepeatFolder
				}
			}
		}

		// Count the number of uncommitted storage folders, and add it to the
		// number of committed storage folders. Folders which are in the
		// process of being removed are not considered in this count. There
		// will only be one uncommitted storage folder with each path, though
		// that storage folder may have multiple entries in the uncommitted
		// changes (ifit has progressed from UnfinishedStorageFolderAdditions
		// to StorageFolderAdditions, for example). A map can therefore be used
		// to determine how many unique storage folders are currently being
		// added.
		uniqueFolders := make(map[string]struct{})
		for _, uc := range wal.uncommittedChanges {
			for _, sfa := range uc.StorageFolderAdditions {
				uniqueFolders[sfa.Path] = struct{}{}
			}
		}
		for _, uc := range wal.uncommittedChanges {
			for _, usfa := range uc.UnfinishedStorageFolderAdditions {
				uniqueFolders[usfa.Path] = struct{}{}
			}
		}
		numStorageFolders += uint64(len(uniqueFolders))
		if numStorageFolders > maximumStorageFolders {
			return errMaxStorageFolders
		}

		// Add the storage folder to the list of unfinished storage folder
		// additions, so that no naming conflicts can appear while this storage
		// folder is being processed.
		//
		// The change is intentionally appended while the progress fields of
		// the storage folder are empty, so that they are empty when they are
		// written to the WAL.
		wal.appendChange(stateChange{
			UnfinishedStorageFolderAdditions: []*storageFolder{sf},
		})

		// Establish the progress fields for the add operation in the storage
		// folder.
		atomic.StoreUint64(&sf.atomicProgressDenominator, totalSize)
		return nil
	}()
	if err != nil {
		return err
	}

	// If there's an error in the rest of the function, the storage folder
	// needs to be removed from the list of unfinished storage folder
	// additions. Because the WAL is append-only, a stateChange needs to be
	// appended which indicates that the storage folder was unable to be added
	// successfully.
	defer func() {
		if err != nil {
			wal.mu.Lock()
			defer wal.mu.Unlock()
			err = build.ComposeErrors(err, wal.appendChange(stateChange{
				ErroredStorageFolderAdditions: []string{sf.Path},
			}))
		}
	}()

	// The WAL now contains a commitment to create the storage folder, but the
	// storage folder still needs to be created. Open a file and write empty
	// data across the whole file to reserve space on disk for sector
	// activities.
	sectorHousingName := filepath.Join(sf.Path, sectorFile)
	file, err := wal.cm.dependencies.createFile(sectorHousingName)
	if err != nil {
		return build.ExtendErr("unable to create storage file for storage folder", err)
	}
	defer file.Close()
	defer func() {
		if err != nil {
			err = build.ComposeErrors(err, os.Remove(sectorHousingName))
		}
	}()
	hundredMBWrites := totalSize / 100e6
	finalWriteSize := totalSize % 100e6
	hundredMB := make([]byte, 100e6)
	finalBytes := make([]byte, finalWriteSize)
	for i := uint64(0); i < hundredMBWrites; i++ {
		_, err = file.Write(hundredMB)
		if err != nil {
			return build.ExtendErr("could not allocate storage folder", err)
		}
		// After each iteration, update the progress numerator.
		atomic.AddUint64(&sf.atomicProgressNumerator, 100e6)
	}
	_, err = file.Write(finalBytes)
	if err != nil {
		return build.ExtendErr("could not allocate storage folder", err)
	}
	err = file.Sync()
	if err != nil {
		return build.ExtendErr("could not syncronize allocated storage folder", err)
	}
	// The file creation process is essentially complete at this point, report
	// complete progress.
	atomic.StoreUint64(&sf.atomicProgressDenominator, totalSize)

	// All of the required setup for the storage folder is complete, add the
	// directive to modify the contract manager state to the WAL, so that the
	// operation can be fully integrated.
	wal.mu.Lock()
	err = wal.appendChange(stateChange{
		StorageFolderAdditions: []*storageFolder{sf},
	})
	// Grab the sync lock and reset the progress values for the storage folder.
	// The values are reset before the lock is released so that conflicting
	// directives do not make overlapping atomic operations.
	syncWait := wal.syncChan
	atomic.StoreUint64(&sf.atomicProgressNumerator, 0)
	atomic.StoreUint64(&sf.atomicProgressDenominator, 0)
	wal.mu.Unlock()
	if err != nil {
		return build.ExtendErr("storage folder commitment assignment failed", err)
	}
	// Only return after the wallet has finished committing, which we can
	// measure by watching the sync chan.
	<-syncWait
	return nil
}

// cleanupUnfinishedStorageFolderAdditions should only be called at startup.
// Any unfinished storage folder additions will be purged from the disk. There
// should only be storage folder additions left behind if there was power loss
// during the setup of a storage folder.
func (wal *writeAheadLog) cleanupUnfinishedStorageFolderAdditions() error {
	for i, uc := range wal.uncommittedChanges {
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
		}
		// Because we're in the middle of startup, the sync loop has not
		// spawned and the changes that we are processing have not been written
		// to disk. That means that we can modify the uncommitted changes
		// directly to remove the actions that are no longer relevant.
		wal.uncommittedChanges[i].UnfinishedStorageFolderAdditions = nil
	}
	return nil
}

// commitAddStorageFolder integrates a pending AddStorageFolder call into the
// state. commitAddStorageFolder should only be called when finalizing an ACID
// transaction, and only after the WAL has been synced to disk, to ensure that
// the state change has been guaranteed even in the event of sudden power loss.
func (wal *writeAheadLog) commitAddStorageFolder(sf *storageFolder) {
	wal.cm.storageFolders = append(wal.cm.storageFolders, sf)
}

// findUnfinishedStorageFolderAdditions will scroll through a set of state
// changes and figure out which of the unfinished storage folder additions are
// still unfinished. If a storage folder addition has finished, it will be
// discoverable through the presence of a storage folder addition object using
// the same path, or through an errored storage folder addition object using
// the same path. Putting the logic here keeps the WAL logic cleaner.
func (wal *writeAheadLog) findUnfinishedStorageFolderAdditions(scs []stateChange) []*storageFolder {
	// Create a map containing a list of all unfinished storage folder
	// additions identified by their path to the storage folder that is being
	// added.
	usfMap := make(map[string]*storageFolder)
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

	// Assemble all of the unfinished storage folder additions that still
	// remain.
	var sfs []*storageFolder
	for _, sf := range usfMap {
		sfs = append(sfs, sf)
	}
	return sfs
}

// AddStorageFolder adds a storage folder to the contract manager.
func (cm *ContractManager) AddStorageFolder(path string, size uint64) error {
	err := cm.tg.Add()
	if err != nil {
		return err
	}
	defer cm.tg.Done()

	// Because the state of the contract manager depends on uncommitted changes
	// that are in the WAL, the state of the contract manager should not be
	// accessed at all inside of this function. Instead, the WAL should take
	// care of all state-related error checking.

	// Check that the storage folder being added meets the size requirements.
	sectors := size / modules.SectorSize
	if sectors > maximumSectorsPerStorageFolder {
		return errLargeStorageFolder
	}
	if sectors < minimumSectorsPerStorageFolder {
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
	newSF := &storageFolder{
		Path:  path,
		Usage: make([]uint32, size/modules.SectorSize/32),
	}
	return cm.wal.managedAddStorageFolder(newSF)
}
