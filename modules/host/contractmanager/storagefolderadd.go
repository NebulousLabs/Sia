package contractmanager

import (
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
)

// findUnfinishedStorageFolderAdditions will scroll through a set of state
// changes and figure out which of the unfinished storage folder additions are
// still unfinished. If a storage folder addition has finished, it will be
// discoverable through the presence of a storage folder addition object using
// the same path, or through an errored storage folder addition object using
// the same path. Putting the logic here keeps the WAL logic cleaner.
func findUnfinishedStorageFolderAdditions(scs []stateChange) []*storageFolder {
	// Create a map containing a list of all unfinished storage folder
	// additions identified by their path to the storage folder that is being
	// added.
	usfMap := make(map[uint16]*storageFolder)
	for _, sc := range scs {
		for _, sf := range sc.UnfinishedStorageFolderAdditions {
			usfMap[sf.Index] = sf
		}
		for _, sf := range sc.StorageFolderAdditions {
			delete(usfMap, sf.Index)
		}
		for _, index := range sc.ErroredStorageFolderAdditions {
			delete(usfMap, index)
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
func (wal *writeAheadLog) managedAddStorageFolder(sf *storageFolder) error {
	// Precompute the total on-disk size of the storage folder. The storage
	// folder needs to have modules.SectorSize available for each sector, plus
	// another 16 bytes per sector to store a mapping from sector id to its
	// location in the storage folder.
	numSectors := uint64(len(sf.Usage)) * 64
	sectorLookupSize := numSectors * 16
	sectorHousingSize := numSectors * modules.SectorSize
	totalSize := sectorLookupSize + sectorHousingSize
	sectorHousingName := filepath.Join(sf.Path, sectorFile)

	// Update the uncommitted state to include the storage folder, returning an
	// error if any checks fail.
	var syncChan chan struct{}
	err := func() error {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		// Check that the storage folder is not a duplicate. That requires
		// first checking the contract manager and then checking the WAL. The
		// number of storage folders are also counted, to make sure that the
		// maximum number of storage folders allowed is not exceeded.
		for _, csf := range wal.cm.storageFolders {
			if sf.Path == csf.Path {
				return errRepeatFolder
			}
		}
		// Check the uncommitted changes for updates to the storage folders
		// which alter the 'duplicate' status of this storage folder.
		for _, uc := range wal.uncommittedChanges {
			for _, usfa := range uc.UnfinishedStorageFolderAdditions {
				if usfa.Path == sf.Path {
					return errRepeatFolder
				}
			}
		}

		// Count the number of uncommitted storage folders, and add it to the
		// number of committed storage folders. This count should include all
		// storage folders being resized or renewed as well.
		uniqueFolders := make(map[uint16]struct{})
		for _, sf := range wal.cm.storageFolders {
			uniqueFolders[sf.Index] = struct{}{}
		}
		for _, uc := range wal.uncommittedChanges {
			// If the unfinished additions have completed, they may be
			// duplicates with the storage folders tracked by the contract
			// managers, which is why the map is used.
			for _, usfa := range uc.UnfinishedStorageFolderAdditions {
				uniqueFolders[usfa.Index] = struct{}{}
			}
		}
		if uint64(len(uniqueFolders)) > maximumStorageFolders {
			return errMaxStorageFolders
		}

		// Determine the index of the storage folder by scanning for an empty
		// spot in the folderLocations map. A random starting place is chosen
		// to keep good average and worst-case O-notation on the runtime for
		// finding an available index.
		var iterator int
		var index uint16
		rand, err := crypto.RandIntn(65536)
		if err != nil {
			wal.cm.log.Critical("no entropy for random iteration when adding a storage folder")
		}
		index = uint16(rand)
		for iterator = 0; iterator < 65536; iterator++ {
			_, exists := wal.cm.storageFolders[index]
			if !exists {
				break
			}
			index++
		}
		if iterator == 65536 {
			wal.cm.log.Critical("Previous check indicated that there was room to add another storage folder, but folderLocations amp is full.")
			return errMaxStorageFolders
		}
		// Assign the empty index to the storage folder.
		sf.Index = index

		// Add the storage folder to the list of unfinished storage folder
		// additions, so that no naming conflicts can appear while this storage
		// folder is being processed.
		wal.appendChange(stateChange{
			UnfinishedStorageFolderAdditions: []*storageFolder{sf},
		})
		// Create the file that is used with the storage folder.
		sf.file, err = wal.cm.dependencies.createFile(sectorHousingName)
		if err != nil {
			return build.ExtendErr("could not create storage folder file", err)
		}
		// Establish the progress fields for the add operation in the storage
		// folder.
		atomic.StoreUint64(&sf.atomicProgressDenominator, totalSize)

		// Grab a sync channel so that we know when the unfinished storage
		// folder addition has been committed to the WAL. Sync chan must be
		// grabbed inside the WAL lock.
		syncChan = wal.syncChan
		return nil
	}()
	if err != nil {
		return err
	}
	// Don't start making files on disk until the unfinished addition has
	// synced, otherwise they will not be cleaned up correctly following an
	// unclean shutdown.
	<-syncChan

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
				ErroredStorageFolderAdditions: []uint16{sf.Index},
			}))
			err = build.ComposeErrors(err, os.Remove(sectorHousingName))
		}
	}()

	// The WAL now contains a commitment to create the storage folder, but the
	// storage folder still needs to be created. Open a file and write empty
	// data across the whole file to reserve space on disk for sector
	// activities.
	writeCount := totalSize / 4e6
	finalWriteSize := totalSize % 4e6
	writeData := make([]byte, 4e6)
	finalBytes := make([]byte, finalWriteSize)
	for i := uint64(0); i < writeCount; i++ {
		_, err = sf.file.Write(writeData)
		if err != nil {
			return build.ExtendErr("could not allocate storage folder", err)
		}
		// After each iteration, update the progress numerator.
		atomic.AddUint64(&sf.atomicProgressNumerator, 4e6)
	}
	_, err = sf.file.Write(finalBytes)
	if err != nil {
		return build.ExtendErr("could not allocate storage folder", err)
	}
	err = sf.file.Sync()
	if err != nil {
		return build.ExtendErr("could not syncronize allocated storage folder", err)
	}
	// The file creation process is essentially complete at this point, report
	// complete progress.
	atomic.StoreUint64(&sf.atomicProgressDenominator, totalSize)

	// Under certain testing scenarious, be disrupted at this point such that
	// AddStorageFolder does not complete, simulating a power-failure while in
	// the middle of adding a storage folder.
	if wal.cm.dependencies.disrupt("incompleteAddStorageFolder") {
		// An error is not returned, as this is simulating a power failure. I'm
		// not certain this is the best thing to return here, but so far it has
		// not caused issues while testing.
		return nil
	}

	// All of the required setup for the storage folder is complete, add the
	// directive to modify the contract manager state to the WAL, so that the
	// operation can be fully integrated.
	wal.mu.Lock()
	wal.cm.storageFolders[sf.Index] = sf
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
// Any unfinished storage folder additions from the previous run will be purged
// from the disk.
func (wal *writeAheadLog) cleanupUnfinishedStorageFolderAdditions(scs []stateChange) error {
	// Some of the input unfinished storage folder additions may have
	// completed. Fetch the set of storage folder additions which are
	// incomplete.
	sfs := findUnfinishedStorageFolderAdditions(scs)
	for _, sf := range sfs {
		// The storage folder addition was interrupted due to an unexpected
		// error, and the change should be aborted. This can be completed by
		// simply removing the file that was partially created to house the
		// sectors that would have appeared in the storage folder.
		sectorHousingName := filepath.Join(sf.Path, sectorFile)
		err := os.Remove(sectorHousingName)
		if err != nil {
			wal.cm.log.Println("Unable to remove documented sector housing:", sectorHousingName, err)
		}

		// Append an error call to the changeset, indicating that the storage
		// folder add was not completed successfully.
		err = wal.appendChange(stateChange{
			ErroredStorageFolderAdditions: []uint16{sf.Index},
		})
		if err != nil {
			return build.ExtendErr("unable to close out unfinished storage folder addition", err)
		}
	}
	return nil
}

// commitAddStorageFolder integrates a pending AddStorageFolder call into the
// state. commitAddStorageFolder should only be called when finalizing an ACID
// transaction, and only after the WAL has been synced to disk, to ensure that
// the state change has been guaranteed even in the event of sudden power loss.
func (wal *writeAheadLog) commitAddStorageFolder(sf *storageFolder) {
	// There is a chance that commitAddStorageFolder gets called multiple
	// times. Especially at startup, that means that there may be an existing
	// storage folder with a file handle already open. If the storage folder
	// already exists, copy over the file handle, otherwise create a new file
	// handle.
	esf, exists := wal.cm.storageFolders[sf.Index]
	if exists {
		sf.file = esf.file
		wal.cm.storageFolders[sf.Index] = sf
	} else {
		var err error
		sf.file, err = os.OpenFile(filepath.Join(sf.Path, sectorFile), os.O_WRONLY, 0700)
		if err != nil {
			sf.FailedReads += 1
			wal.cm.log.Println("Difficulties opening storage folder:", err)
		}
		wal.cm.storageFolders[sf.Index] = sf
	}
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
		Usage: make([]uint64, size/modules.SectorSize/64),
	}
	return cm.wal.managedAddStorageFolder(newSF)
}
