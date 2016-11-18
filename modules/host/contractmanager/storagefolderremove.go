package contractmanager

import (
	"path/filepath"
)

type (
	// storageFolderRemoval indicates a storage folder that has been removed
	// from the WAL.
	storageFolderRemoval struct {
		Index uint16
		Path  string
	}
)

// commitStorageFolderRemoval will finalize a storage folder removal from the
// contract manager.
func (wal *writeAheadLog) commitStorageFolderRemoval(sfr storageFolderRemoval) {
	// Close any open file handles.
	sf, exists := wal.cm.storageFolders[sfr.Index]
	if exists {
		delete(wal.cm.storageFolders, sfr.Index)
	}
	if exists && sf.metadataFile != nil {
		err := sf.metadataFile.Close()
		if err != nil {
			wal.cm.log.Printf("Error: unable to close metadata file as storage folder %v is removed\n", sf.path)
		}
	}
	if exists && sf.sectorFile != nil {
		err := sf.sectorFile.Close()
		if err != nil {
			wal.cm.log.Printf("Error: unable to close sector file as storage folder %v is removed\n", sf.path)
		}
	}

	// Delete the files.
	err := wal.cm.dependencies.removeFile(filepath.Join(sfr.Path, metadataFile))
	if err != nil {
		wal.cm.log.Printf("Error: unable to remove metadata file as storage folder %v is removed\n", sfr.Path)
	}
	err = wal.cm.dependencies.removeFile(filepath.Join(sfr.Path, sectorFile))
	if err != nil {
		wal.cm.log.Printf("Error: unable to reomve sector file as storage folder %v is removed\n", sfr.Path)
	}
}

// RemoveStorageFolder will delete a storage folder from the contract manager,
// moving all of the sectors in the storage folder to new storage folders.
func (cm *ContractManager) RemoveStorageFolder(index uint16, force bool) error {
	cm.tg.Add()
	defer cm.tg.Done()

	// Retrieve the specified storage folder.
	cm.wal.mu.Lock()
	sf, exists := cm.storageFolders[index]
	if !exists {
		cm.wal.mu.Unlock()
		return errStorageFolderNotFound
	}
	cm.wal.mu.Unlock()

	// Lock the storage folder for the duration of the operation.
	sf.mu.Lock()
	defer sf.mu.Unlock()

	// Clear out the sectors in the storage folder.
	_, err := cm.wal.managedEmptyStorageFolder(index, 0)
	if err != nil && !force {
		return err
	}

	// Wait for a synchronize to confirm that all of the moves have succeeded
	// in full.
	cm.wal.mu.Lock()
	syncChan := cm.wal.syncChan
	cm.wal.mu.Unlock()
	<-syncChan

	// Submit a storage folder removal to the WAL and wait until the update is
	// synced.
	cm.wal.mu.Lock()
	cm.wal.appendChange(stateChange{
		StorageFolderRemovals: []storageFolderRemoval{{
			Index: index,
			Path:  sf.path,
		}},
	})

	// Wait until the removal action has been synchronized.
	syncChan = cm.wal.syncChan
	cm.wal.mu.Unlock()
	<-syncChan
	return nil
}
