package contractmanager

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
)

type (
	// sectorUpdate is an idempotent update to the sector metadata.
	sectorUpdate struct {
		Count  uint16
		Folder uint16
		ID     sectorID
		Index  uint32
	}

	// stateChange defines an idempotent change to the state that has not yet
	// been applied to the contract manager. The state change is a single
	// transaction in the WAL.
	//
	// All changes in the stateChange object need to be idempotent, as it's
	// possible that consecutive unclean shutdowns will result in changes being
	// committed to the state multiple times.
	stateChange struct {
		// These fields relate to adding a storage folder. Adding a storage
		// folder happens in several stages.
		//
		// First the storage folder is added as an
		// 'UnfinishedStorageFolderAddition', because there is large amount of
		// I/O preprocessing that is performed when adding a storage folder.
		// This I/O must be nonblocking and must resume in the event of unclean
		// or early shutdown.
		//
		// When the preprocessing is complete, the storage folder is moved to a
		// 'StorageFolderAddition', which can be safely applied to the contract
		// manager but hasn't yet.
		//
		// ErroredStorageFolderAdditions are signals to the WAL that an
		// unfinished storage folder addition has failed and can be cleared
		// out. The WAL is append-only, which is why an error needs to be
		// logged instead of just automatically clearning out the unfinished
		// storage folder addition.
		ErroredStorageFolderAdditions     []uint16
		ErroredStorageFolderExtensions    []uint16
		StorageFolderAdditions            []savedStorageFolder
		StorageFolderExtensions           []storageFolderExtension
		StorageFolderRemovals             []storageFolderRemoval
		StorageFolderReductions           []storageFolderReduction
		UnfinishedStorageFolderAdditions  []savedStorageFolder
		UnfinishedStorageFolderExtensions []unfinishedStorageFolderExtension

		// Updates to the sector metadata. Careful ordering of events ensures
		// that a sector update will not make it into the synced WAL unless the
		// sector data is already on-disk and synced.
		SectorUpdates []sectorUpdate
	}

	// writeAheadLog coordinates ACID transactions which update the state of
	// the contract manager. Consistency on a field is only guaranteed by
	// looking it up through the WAL, and is not guaranteed by direct access.
	writeAheadLog struct {
		// The primary feature of the WAL is a file on disk that records all of
		// the changes which have been proposed. The data is written to a temp
		// file and then renamed atomically to a non-corrupt commitment of
		// actions to be committed to the state. Data is written to the temp
		// file continuously for performance reasons - when a Sync() ->
		// Rename() occurs, most of the data will have already been flushed to
		// disk, making the operation faster. The same is done with the
		// settings file, which might be multiple MiB large for larger storage
		// arrays.
		//
		// To further increase throughput, the WAL will batch as many
		// operations as possible. These operations can happen concurrently,
		// and will block until the contract manager can provide an ACID
		// guarantee that the operation has completed. Syncing of multiple
		// operations happens all at once, and the syncChan is used to signal
		// that a sync operation has completed, providing ACID guarantees to
		// any operation waiting on it. The mechanism of announcing is to close
		// the syncChan, and then to create a new one for new operations to
		// listen on.
		//
		// uncommittedChanges details a list of operations which have been
		// suggested or queued to be made to the state, but are not yet
		// guaranteed to have completed.
		fileSettingsTmp    modules.File
		fileWALTmp         modules.File
		syncChan           chan struct{}
		uncommittedChanges []stateChange
		committedSettings  savedSettings

		// Utilities. The WAL needs access to the ContractManager because all
		// mutations to ACID fields of the contract manager happen through the
		// WAL.
		cm *ContractManager
		mu sync.Mutex
	}
)

// readWALMetadata reads WAL metadata from the input file, returning an error
// if the result is unexpected.
func readWALMetadata(decoder *json.Decoder) error {
	var md persist.Metadata
	err := decoder.Decode(&md)
	if err != nil {
		return build.ExtendErr("error reading WAL metadata", err)
	}
	if md.Header != walMetadata.Header {
		return errors.New("WAL metadata header does not match header found in WAL file")
	}
	if md.Version != walMetadata.Version {
		return errors.New("WAL metadata version does not match version found in WAL file")
	}
	return nil
}

// writeWALMetadata writes WAL metadata to the input file.
func writeWALMetadata(f modules.File) error {
	changeBytes, err := json.MarshalIndent(walMetadata, "", "\t")
	if err != nil {
		return build.ExtendErr("could not marshal WAL metadata", err)
	}
	_, err = f.Write(changeBytes)
	if err != nil {
		return build.ExtendErr("unable to write WAL metadata", err)
	}
	return nil
}

// appendChange will add a change to the WAL, writing the details of the change
// to the WAL file but not syncing - syncing is orchestrated by the sync loop.
//
// The WAL is append only, which means that changes can only be revoked by
// appending an error. This is common for long running operations like adding a
// storage folder.
func (wal *writeAheadLog) appendChange(sc stateChange) {
	// Marshal the change and then write the change to the WAL file. Syncing
	// happens in the sync loop.
	changeBytes, err := json.MarshalIndent(sc, "", "\t")
	if err != nil {
		wal.cm.log.Severe("Unable to marshal state change:", err)
		panic("unable to append a change to the WAL, crashing to prevent corruption")
	}
	_, err = wal.fileWALTmp.Write(changeBytes)
	if err != nil {
		wal.cm.log.Severe("Unable to write state change to WAL:", err)
		panic("unable to append a change to the WAL, crashing to prevent corruption")
	}

	// Update the WAL to include the new storage folder in the uncommitted
	// changes.
	wal.uncommittedChanges = append(wal.uncommittedChanges, sc)
}

// commitChange will commit the provided change to the contract manager,
// updating both the in-memory state and the on-disk state.
//
// It should be noted that long running tasks are ignored during calls to
// commitChange, as they haven't completed and are being managed by a separate
// thread. Upon completion, they will be converted into a different type of
// commitment.
func (wal *writeAheadLog) commitChange(sc stateChange) {
	for _, sfa := range sc.StorageFolderAdditions {
		for i := uint64(0); i < wal.cm.dependencies.AtLeastOne(); i++ {
			wal.commitAddStorageFolder(sfa)
		}
	}
	for _, sfe := range sc.StorageFolderExtensions {
		for i := uint64(0); i < wal.cm.dependencies.AtLeastOne(); i++ {
			wal.commitStorageFolderExtension(sfe)
		}
	}
	for _, sfr := range sc.StorageFolderReductions {
		for i := uint64(0); i < wal.cm.dependencies.AtLeastOne(); i++ {
			wal.commitStorageFolderReduction(sfr)
		}
	}
	for _, sfr := range sc.StorageFolderRemovals {
		for i := uint64(0); i < wal.cm.dependencies.AtLeastOne(); i++ {
			wal.commitStorageFolderRemoval(sfr)
		}
	}
	for _, su := range sc.SectorUpdates {
		for i := uint64(0); i < wal.cm.dependencies.AtLeastOne(); i++ {
			wal.commitUpdateSector(su)
		}
	}
}

// createWALTmp will open up the temporary WAL file.
func (wal *writeAheadLog) createWALTmp() {
	var err error
	walTmpName := filepath.Join(wal.cm.persistDir, walFileTmp)
	wal.fileWALTmp, err = wal.cm.dependencies.CreateFile(walTmpName)
	if err != nil {
		wal.cm.log.Severe("Unable to create WAL temporary file:", err)
		panic("unable to create WAL temporary file, crashing to avoid corruption")
	}
	err = writeWALMetadata(wal.fileWALTmp)
	if err != nil {
		wal.cm.log.Severe("Unable to write WAL metadata:", err)
		panic("unable to create WAL temporary file, crashing to prevent corruption")
	}
}

// recoverWAL will read a previous WAL and re-commit all of the changes inside,
// restoring the program to consistency after an unclean shutdown. The tmp WAL
// file needs to be open before this function is called.
func (wal *writeAheadLog) recoverWAL(walFile modules.File) error {
	// Read the WAL metadata to make sure that the version is correct.
	decoder := json.NewDecoder(walFile)
	err := readWALMetadata(decoder)
	if err != nil {
		wal.cm.log.Println("ERROR: error while reading WAL metadata:", err)
		return build.ExtendErr("walFile metadata mismatch", err)
	}

	// Read changes from the WAL one at a time and load them back into memory.
	// A full list of changes is kept so that modifications to long running
	// changes can be parsed properly.
	var sc stateChange
	var scs []stateChange
	for err == nil {
		err = decoder.Decode(&sc)
		if err == nil {
			// The uncommitted changes are loaded into memory using a simple
			// append, because the tmp WAL file has not been created yet, and
			// will not be created until the sync loop is spawned. The sync
			// loop spawner will make sure that the uncommitted changes are
			// written to the tmp WAL file.
			wal.commitChange(sc)
			scs = append(scs, sc)
		}
	}
	if err != io.EOF {
		wal.cm.log.Println("ERROR: could not load WAL json:", err)
		return build.ExtendErr("error loading WAL json", err)
	}

	// Do any cleanup regarding long-running unfinished tasks. Long running
	// task cleanup cannot be handled in the 'commitChange' loop because future
	// state changes may indicate that the long running task has actually been
	// completed.
	wal.cleanupUnfinishedStorageFolderAdditions(scs)
	wal.cleanupUnfinishedStorageFolderExtensions(scs)
	return nil
}

// load will pull any changes from the uncommitted WAL into memory, decoding
// them and doing any necessary preprocessing. In the most common case (any
// time the previous shutdown was clean), there will not be a WAL file.
func (wal *writeAheadLog) load() error {
	// Create the walTmpFile, which needs to be open before recovery can start.
	wal.createWALTmp()

	// Close the WAL tmp file upon shutdown.
	wal.cm.tg.AfterStop(func() {
		wal.mu.Lock()
		defer wal.mu.Unlock()

		err := wal.fileWALTmp.Close()
		if err != nil {
			wal.cm.log.Println("ERROR: error closing wal file during contract manager shutdown:", err)
			return
		}
		err = wal.cm.dependencies.RemoveFile(filepath.Join(wal.cm.persistDir, walFileTmp))
		if err != nil {
			wal.cm.log.Println("ERROR: error removing temporary WAL during contract manager shutdown:", err)
			return
		}
	})

	// Try opening the WAL file.
	walFileName := filepath.Join(wal.cm.persistDir, walFile)
	walFile, err := wal.cm.dependencies.OpenFile(walFileName, os.O_RDONLY, 0600)
	if err == nil {
		// err == nil indicates that there is a WAL file, which means that the
		// previous shutdown was not clean. Re-commit the changes in the WAL to
		// bring the program back to consistency.
		wal.cm.log.Println("WARN: WAL file detected, performing recovery after unclean shutdown.")
		err = wal.recoverWAL(walFile)
		if err != nil {
			return build.ExtendErr("failed to recover WAL", err)
		}
		err = walFile.Close()
		if err != nil {
			return build.ExtendErr("error closing WAL after performing a recovery", err)
		}
	} else if !os.IsNotExist(err) {
		return build.ExtendErr("walFile was not opened successfully", err)
	}
	// err == os.IsNotExist, suggesting a successful, clean shutdown. No action
	// is taken.

	// Create the tmp settings file and initialize the first write to it. This
	// is necessary before kicking off the sync loop.
	wal.fileSettingsTmp, err = wal.cm.dependencies.CreateFile(filepath.Join(wal.cm.persistDir, settingsFileTmp))
	if err != nil {
		return build.ExtendErr("unable to prepare the settings temp file", err)
	}
	wal.cm.tg.AfterStop(func() {
		wal.mu.Lock()
		defer wal.mu.Unlock()
		if wal.fileSettingsTmp == nil {
			return
		}
		err := wal.fileSettingsTmp.Close()
		if err != nil {
			wal.cm.log.Println("ERROR: unable to close settings temporary file")
			return
		}
		err = wal.cm.dependencies.RemoveFile(filepath.Join(wal.cm.persistDir, settingsFileTmp))
		if err != nil {
			wal.cm.log.Println("ERROR: unable to remove settings temporary file")
			return
		}
	})
	ss := wal.cm.savedSettings()
	b, err := json.MarshalIndent(ss, "", "\t")
	if err != nil {
		build.ExtendErr("unable to marshal settings data", err)
	}
	enc := json.NewEncoder(wal.fileSettingsTmp)
	if err := enc.Encode(settingsMetadata.Header); err != nil {
		build.ExtendErr("unable to write header to settings temp file", err)
	}
	if err := enc.Encode(settingsMetadata.Version); err != nil {
		build.ExtendErr("unable to write version to settings temp file", err)
	}
	if _, err = wal.fileSettingsTmp.Write(b); err != nil {
		build.ExtendErr("unable to write data settings temp file", err)
	}
	return nil
}
