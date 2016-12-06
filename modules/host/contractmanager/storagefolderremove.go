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

// commitRemoveStorageFolder will finalize a storage folder removal from the
// contract manager.
func (wal *writeAheadLog) commitRemoveStorageFolder(sfr storageFolderRemoval) {
	// Close any open file handles.
	sf, exists := wal.cm.storageFolders[sfr.Index]
	if exists {
		sf.metadataFile.Close()
		sf.sectorFile.Close()
	}

	// Delete the files.
	wal.cm.dependencies.removeFile(filepath.Join(sfr.Path, metadataFile))
	wal.cm.dependencies.removeFile(filepath.Join(sfr.Path, sectorFile))
	delete(wal.cm.storageFolders, sfr.Index)
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
	delete(cm.storageFolders, index)

	// Wait until the removal action has been synchronized.
	syncChan = cm.wal.syncChan
	cm.wal.mu.Unlock()
	<-syncChan

	// Remove the storage folder. Close all handles, and remove the files from
	// disk.
	//
	// TODO: In the rare event that this doesn't happen until after the deleted
	// cm.storageFolders settings update has synchronized, clutter may be left
	// on disk.
	//
	// TODO: Handle these by doing them during the WAL commit.
	err = sf.metadataFile.Close()
	if err != nil {
		cm.log.Printf("Error: unable to close metadata file as storage folder %v is removed\n", sf.path)
	}
	err = sf.sectorFile.Close()
	if err != nil {
		cm.log.Printf("Error: unable to close sector file as storage folder %v is removed\n", sf.path)
	}
	err = cm.dependencies.removeFile(filepath.Join(sf.path, metadataFile))
	if err != nil {
		cm.log.Printf("Error: unable to remove metadata file as storage folder %v is removed\n", sf.path)
	}
	err = cm.dependencies.removeFile(filepath.Join(sf.path, sectorFile))
	if err != nil {
		cm.log.Printf("Error: unable to reomve sector file as storage folder %v is removed\n", sf.path)
	}
	return nil
}
