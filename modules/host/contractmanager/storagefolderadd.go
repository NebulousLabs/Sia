package contractmanager

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/fastrand"
)

// findUnfinishedStorageFolderAdditions will scroll through a set of state
// changes and figure out which of the unfinished storage folder additions are
// still unfinished.
func findUnfinishedStorageFolderAdditions(scs []stateChange) []savedStorageFolder {
	// Use a map to figure out what unfinished storage folders exist and use it
	// to remove the ones that have terminated.
	usfMap := make(map[uint16]savedStorageFolder)
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
		for _, sfr := range sc.StorageFolderRemovals {
			delete(usfMap, sfr.Index)
		}
	}

	// Return the active unifinished storage folders as a slice.
	var sfs []savedStorageFolder
	for _, sf := range usfMap {
		sfs = append(sfs, sf)
	}
	return sfs
}

// cleanupUnfinishedStorageFolderAdditions will purge any unfinished storage
// folder additions from the previous run.
func (wal *writeAheadLog) cleanupUnfinishedStorageFolderAdditions(scs []stateChange) {
	usfs := findUnfinishedStorageFolderAdditions(scs)
	for _, usf := range usfs {
		sf, exists := wal.cm.storageFolders[usf.Index]
		if exists && atomic.LoadUint64(&sf.atomicUnavailable) == 0 {
			// Close the storage folder file handles.
			err := sf.metadataFile.Close()
			if err != nil {
				wal.cm.log.Println("Unable to close metadata file for storage folder", sf.path)
			}
			err = sf.sectorFile.Close()
			if err != nil {
				wal.cm.log.Println("Unable to close sector file for storage folder", sf.path)
			}

			// Delete the storage folder from the storage folders map.
			delete(wal.cm.storageFolders, sf.index)
		}

		// Remove any leftover files.
		sectorLookupName := filepath.Join(usf.Path, metadataFile)
		sectorHousingName := filepath.Join(usf.Path, sectorFile)
		err := wal.cm.dependencies.RemoveFile(sectorLookupName)
		if err != nil {
			wal.cm.log.Println("Unable to remove documented sector metadata lookup:", sectorLookupName, err)
		}
		err = wal.cm.dependencies.RemoveFile(sectorHousingName)
		if err != nil {
			wal.cm.log.Println("Unable to remove documented sector housing:", sectorHousingName, err)
		}

		// Append an error call to the changeset, indicating that the storage
		// folder add was not completed successfully.
		wal.appendChange(stateChange{
			ErroredStorageFolderAdditions: []uint16{usf.Index},
		})
	}
}

// managedAddStorageFolder will add a storage folder to the contract manager.
// The parent function, contractmanager.AddStorageFolder, has already performed
// any error checking that can be performed without accessing the contract
// manager state.
//
// managedAddStorageFolder can take a long time, as it writes a giant, zeroed
// out file to disk covering the entire range of the storage folder, and
// failure can occur late in the operation. The WAL is notified that a long
// running operation is in progress, so that any changes to disk can be
// reverted in the event of unclean shutdown.
func (wal *writeAheadLog) managedAddStorageFolder(sf *storageFolder) error {
	// Lock the storage folder for the duration of the function.
	sf.mu.Lock()
	defer sf.mu.Unlock()

	numSectors := uint64(len(sf.usage)) * 64
	sectorLookupSize := numSectors * sectorMetadataDiskSize
	sectorHousingSize := numSectors * modules.SectorSize
	totalSize := sectorLookupSize + sectorHousingSize
	sectorLookupName := filepath.Join(sf.path, metadataFile)
	sectorHousingName := filepath.Join(sf.path, sectorFile)

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
			// The conflicting storage folder may e in the process of being
			// removed, however we refuse to add a replacement storage folder
			// until the existing one has been removed entirely.
			if sf.path == csf.path {
				return ErrRepeatFolder
			}
		}

		// Check that there is room for another storage folder.
		if uint64(len(wal.cm.storageFolders)) > maximumStorageFolders {
			return errMaxStorageFolders
		}

		// Determine the index of the storage folder by scanning for an empty
		// spot in the folderLocations map. A random starting place is chosen
		// to keep good average and worst-case runtime.
		var iterator int
		index := uint16(fastrand.Intn(65536))
		for iterator = 0; iterator < 65536; iterator++ {
			// check the list of unique folders we created earlier.
			_, exists := wal.cm.storageFolders[index]
			if !exists {
				break
			}
			index++
		}
		if iterator == 65536 {
			wal.cm.log.Critical("Previous check indicated that there was room to add another storage folder, but folderLocations set is full.")
			return errMaxStorageFolders
		}
		// Assign the empty index to the storage folder.
		sf.index = index

		// Create the files that get used with the storage folder.
		var err error
		sf.metadataFile, err = wal.cm.dependencies.CreateFile(sectorLookupName)
		if err != nil {
			return build.ExtendErr("could not create storage folder file", err)
		}
		sf.sectorFile, err = wal.cm.dependencies.CreateFile(sectorHousingName)
		if err != nil {
			err = build.ComposeErrors(err, sf.metadataFile.Close())
			err = build.ComposeErrors(err, wal.cm.dependencies.RemoveFile(sectorLookupName))
			return build.ExtendErr("could not create storage folder file", err)
		}
		// Establish the progress fields for the add operation in the storage
		// folder.
		atomic.StoreUint64(&sf.atomicProgressDenominator, totalSize)

		// Add the storage folder to the list of storage folders.
		wal.cm.storageFolders[index] = sf

		// Add the storage folder to the list of unfinished storage folder
		// additions. There should be no chance of error between this append
		// operation and the completed commitment to the unfinished storage
		// folder addition (signaled by `<-syncChan` a few lines down).
		wal.appendChange(stateChange{
			UnfinishedStorageFolderAdditions: []savedStorageFolder{sf.savedStorageFolder()},
		})
		// Grab the sync channel so we know when the unfinished storage folder
		// addition has been committed to on disk.
		syncChan = wal.syncChan
		return nil
	}()
	if err != nil {
		return err
	}
	// Block until the commitment to the unfinished storage folder addition is
	// complete.
	<-syncChan

	// Simulate a disk failure at this point.
	if wal.cm.dependencies.Disrupt("storageFolderAddFinish") {
		return nil
	}

	// If there's an error in the rest of the function, the storage folder
	// needs to be removed from the list of unfinished storage folder
	// additions. Because the WAL is append-only, a stateChange needs to be
	// appended which indicates that the storage folder was unable to be added
	// successfully.
	defer func(sf *storageFolder) {
		if err != nil {
			wal.mu.Lock()
			defer wal.mu.Unlock()

			// Delete the storage folder from the storage folders map.
			delete(wal.cm.storageFolders, sf.index)

			// Remove the leftover files from the failed operation.
			err = build.ComposeErrors(err, sf.sectorFile.Close())
			err = build.ComposeErrors(err, sf.metadataFile.Close())
			err = build.ComposeErrors(err, wal.cm.dependencies.RemoveFile(sectorLookupName))
			err = build.ComposeErrors(err, wal.cm.dependencies.RemoveFile(sectorHousingName))

			// Signal in the WAL that the unfinished storage folder addition
			// has failed.
			wal.appendChange(stateChange{
				ErroredStorageFolderAdditions: []uint16{sf.index},
			})
		}
	}(sf)

	// Allocate the files on disk for the storage folder.
	stepCount := sectorHousingSize / folderAllocationStepSize
	for i := uint64(0); i < stepCount; i++ {
		err = sf.sectorFile.Truncate(int64(folderAllocationStepSize * (i + 1)))
		if err != nil {
			return build.ExtendErr("could not allocate storage folder", err)
		}
		// After each iteration, update the progress numerator.
		atomic.AddUint64(&sf.atomicProgressNumerator, folderAllocationStepSize)
	}
	err = sf.sectorFile.Truncate(int64(sectorHousingSize))
	if err != nil {
		return build.ExtendErr("could not allocate sector data file", err)
	}

	// Write the metadata file.
	err = sf.metadataFile.Truncate(int64(sectorLookupSize))
	if err != nil {
		return build.ExtendErr("could not allocate sector metadata file", err)
	}

	// The file creation process is essentially complete at this point, report
	// complete progress.
	atomic.StoreUint64(&sf.atomicProgressNumerator, totalSize)

	// Sync the files.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := sf.metadataFile.Sync()
		if err != nil {
			wal.cm.log.Println("could not synchronize allocated sector metadata file:", err)
		}
	}()
	go func() {
		defer wg.Done()
		err := sf.sectorFile.Sync()
		if err != nil {
			wal.cm.log.Println("could not synchronize allocated sector data file:", err)
		}
	}()
	wg.Wait()

	// TODO: Sync the directory as well (directory data changed as new files
	// were added)

	// Simulate power failure at this point for some testing scenarios.
	if wal.cm.dependencies.Disrupt("incompleteAddStorageFolder") {
		return nil
	}

	// Storage folder addition has completed successfully, commit the addition
	// through the WAL.
	wal.mu.Lock()
	wal.cm.storageFolders[sf.index] = sf
	wal.appendChange(stateChange{
		StorageFolderAdditions: []savedStorageFolder{sf.savedStorageFolder()},
	})
	syncChan = wal.syncChan
	wal.mu.Unlock()

	// Wait to confirm the storage folder addition has completed until the WAL
	// entry has synced.
	<-syncChan

	// Set the progress back to '0'.
	atomic.StoreUint64(&sf.atomicProgressNumerator, 0)
	atomic.StoreUint64(&sf.atomicProgressDenominator, 0)
	return nil
}

// commitAddStorageFolder integrates a pending AddStorageFolder call into the
// state. commitAddStorageFolder should only be called during WAL recovery.
func (wal *writeAheadLog) commitAddStorageFolder(ssf savedStorageFolder) {
	sf, exists := wal.cm.storageFolders[ssf.Index]
	if exists {
		if sf.metadataFile != nil {
			sf.metadataFile.Close()
		}
		if sf.sectorFile != nil {
			sf.sectorFile.Close()
		}
	}

	sf = &storageFolder{
		index: ssf.Index,
		path:  ssf.Path,
		usage: ssf.Usage,

		availableSectors: make(map[sectorID]uint32),
	}

	var err error
	sf.metadataFile, err = wal.cm.dependencies.OpenFile(filepath.Join(sf.path, metadataFile), os.O_RDWR, 0700)
	if err != nil {
		wal.cm.log.Println("Difficulties opening sector file for ", sf.path, ":", err)
		return
	}
	sf.sectorFile, err = wal.cm.dependencies.OpenFile(filepath.Join(sf.path, sectorFile), os.O_RDWR, 0700)
	if err != nil {
		wal.cm.log.Println("Difficulties opening sector metadata file for", sf.path, ":", err)
		sf.metadataFile.Close()
		return
	}
	wal.cm.storageFolders[sf.index] = sf
}

// AddStorageFolder adds a storage folder to the contract manager.
func (cm *ContractManager) AddStorageFolder(path string, size uint64) error {
	err := cm.tg.Add()
	if err != nil {
		return err
	}
	defer cm.tg.Done()

	// Check that the storage folder being added meets the size requirements.
	sectors := size / modules.SectorSize
	if sectors > MaximumSectorsPerStorageFolder {
		return ErrLargeStorageFolder
	}
	if sectors < MinimumSectorsPerStorageFolder {
		return ErrSmallStorageFolder
	}
	if sectors%storageFolderGranularity != 0 {
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
		path:  path,
		usage: make([]uint64, sectors/64),

		availableSectors: make(map[sectorID]uint32),
	}
	err = cm.wal.managedAddStorageFolder(newSF)
	if err != nil {
		cm.log.Println("Call to AddStorageFolder has failed:", err)
		return err
	}
	return nil
}
