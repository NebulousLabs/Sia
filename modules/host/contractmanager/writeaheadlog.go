package contractmanager

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/persist"
)

type (
	// TODO: For greatly improved space efficiency, and also probably encoding
	// time efficiency, make a custom json marshaller that can pack this stuff.
	// The big killer is 'Data', which is often going to be 4 MiB large.
	sectorAdd struct {
		Count  uint16
		Data   []byte
		Folder uint16
		ID     sectorID
		Index  uint32
	}

	// stateChange defines a change to the state that has not yet been applied
	// to the contract manager, but will be applied in a future commitment.
	//
	// All changes in the stateChange object need to be idempotent, as it's
	// possible that multiple corruptions or failures will result in the
	// changes being committed to the state multiple times.
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
		ErroredStorageFolderAdditions []uint16
		StorageFolderAdditions        []*storageFolder

		AddedSectors []sectorAdd

		// These fields all correspond to long-running actions in the WAL. The
		// storage folder additions must be documented so that there are no
		// conflicts when performing multiple actions in parallel, but they are
		// not also processed at commit() time because preparation may not have
		// completed when the commit loop fires. Instead, other actions will
		// indicate that they have completed and that can be used to trigger
		// cleanup.
		UnfinishedStorageFolderAdditions []*storageFolder
	}

	// writeAheadLog coordinates ACID transactions which update the state of
	// the contract manager. WAL should be the only entity that is accessing
	// the contract manager's mutex.
	writeAheadLog struct {
		// The file is being constantly written to. Sync operations are very
		// slow, but they are faster if most of the data has already had time
		// to hit the disk. Rather than writing the entire WAL to disk every
		// time there is a desired sync operation, the WAL is continuously
		// writing to disk which should make the sync and commit operations a
		// lot faster.
		//
		// The syncChan lets external callers know when an operation that they
		// have requested has been completed and successfully, consistently,
		// and durably hit the disk. Multiple callers can be listening on the
		// syncChan, so to send them all the same consistent message, the
		// channel is closed every time a commit operation finishes. After the
		// channel is closed, it is deleted and a new one is created.
		//
		// uncommittedChanges details a list of operations which have been
		// suggested or queued to be made to the state, but are not yet
		// guaranteed to have completed.
		fileSettingsTmp    *os.File
		fileWALTmp         *os.File
		syncChan           chan struct{}
		uncommittedChanges []stateChange

		// Utilities. The WAL needs access to the ContractManager because all
		// mutations to ACID fields of the contract manager happen through the
		// WAL.
		cm           *ContractManager
		loadComplete bool
		mu           sync.Mutex
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
func writeWALMetadata(f *os.File) error {
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
// to the WAL file but not syncing - the syncing will occur during the sync and
// commit operations that are orchestrated by the sync loop. Waiting to perform
// the syncing provides a parallelism across multiple disks, and gives the
// operating system time to optimize the operations that will be performed on
// disk, greatly improving performance, especially for random writes.
//
// Once a change is appended, it can only be revoked by appending another
// change. If a long running operation such as the addition or removal of a
// storage folder is occurring, they will need to use multiple types of
// stateChange directives to ensure safety of the operation. An alternative
// method would be to hold a lock on the WAL for the entirety of the operation,
// but this would too greatly impact performance.
func (wal *writeAheadLog) appendChange(sc stateChange) error {
	// Marshal the change and then write the change to the WAL file. Do not
	// sync the WAL file, as this operation does not need to guarantee that the
	// data hits the platter, the syncLoop will handle that piece.
	changeBytes, err := json.MarshalIndent(sc, "", "\t")
	if err != nil {
		return build.ExtendErr("could not marshal state change", err)
	}
	_, err = wal.fileWALTmp.Write(changeBytes)
	if err != nil {
		return build.ExtendErr("unable to write state change to WAL", err)
	}

	// Update the WAL to include the new storage folder in the uncommitted
	// changes.
	wal.uncommittedChanges = append(wal.uncommittedChanges, sc)
	return nil
}

// applyChange will apply the provided change to the contract manager, updating
// both the in-memory state and the on-disk state.
//
// It should be noted that long running tasks are ignored during calls to
// applyChange, because future changes may indicate that the long running task
// has completed. Long running tasks are started and maintained by separate
// threads, and in the event of power-loss, the long running changes will be
// resumed or cleaned up following a full reloading and synchronization of the
// existing changes.
func (wal *writeAheadLog) applyChange(sc stateChange) {
	for _, sfa := range sc.StorageFolderAdditions {
		for i := uint64(0); i < wal.cm.dependencies.atLeastOne(); i++ {
			wal.commitAddStorageFolder(sfa)
		}
	}
	for _, as := range sc.AddedSectors {
		for i := uint64(0); i < wal.cm.dependencies.atLeastOne(); i++ {
			wal.commitAddSector(as)
		}
	}
}

// createWALTmp will open up the temporary WAL file.
func (wal *writeAheadLog) createWALTmp() (err error) {
	walTmpName := filepath.Join(wal.cm.persistDir, walFileTmp)
	wal.fileWALTmp, err = os.Create(walTmpName)
	if err != nil {
		return build.ExtendErr("unable to create WAL temporary file", err)
	}
	err = writeWALMetadata(wal.fileWALTmp)
	if err != nil {
		return build.ExtendErr("unable to write WAL metadata", err)
	}
	return nil
}

// recoverWAL will read a previous WAL and re-commit all of the changes inside,
// restoring the program to consistency after an unclean shutdown.
func (wal *writeAheadLog) recoverWAL(walFile file) error {
	// Read the WAL metadata to make sure that the version is correct.
	decoder := json.NewDecoder(walFile)
	err := readWALMetadata(decoder)
	if err != nil {
		return build.ExtendErr("walFile metadata mismatch", err)
	}

	// Read changes from the WAL one at a time and load them back into memory.
	// A full list of changes is kept so that long running changes can be
	// processed properly - long running changes are dependent on changes that
	// happen in the future.
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
			wal.applyChange(sc)
			scs = append(scs, sc)
		}
	}
	if err != io.EOF {
		return build.ExtendErr("error loading WAL json", err)
	}

	// Do any cleanup regarding long-running unfinished tasks. Long running
	// task cleanup cannot be handled in the 'applyChange' loop because future
	// state changes may indicate that the long running task has actually been
	// completed.
	//
	// These functions can assume that the tmp wal file is open, because it
	// should have been created before WAL recovery started.
	err = wal.cleanupUnfinishedStorageFolderAdditions(scs)
	if err != nil {
		return build.ExtendErr("error performing unfinished storage folder cleanup", err)
	}
	return nil
}

// load will pull any changes from the uncommited WAL into memory, decoding
// them and doing any necessary preprocessing. In the most common case (any
// time the previous shutdown was clean), there will not be a WAL file.
func (wal *writeAheadLog) load() error {
	// Create the walTmpFile and write all of the remaining long running
	// changes to it.
	err := wal.createWALTmp()
	if err != nil {
		return err
	}

	// Try opening the WAL file.
	walFileName := filepath.Join(wal.cm.persistDir, walFile)
	walFile, err := os.Open(walFileName)
	if err == nil {
		// err == nil indicates that there is a WAL file, which means that the
		// previous shutdown was not clean. Re-commit the changes in the WAL to
		// bring the program back to consistency.
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
	// Final option is that err == os.IsNotExist, which means that there is no
	// WAL, which most likely means that there was a clean shutdown previously.
	// No action needs to be taken regarding WAL recovery.

	// Open up the settings tmp file and write to it. This will finish
	// preparations and allow the sync loop to get started without hiccups.
	wal.fileSettingsTmp, err = os.Create(filepath.Join(wal.cm.persistDir, settingsFileTmp))
	if err != nil {
		return build.ExtendErr("unable to prepare the settings temp file", err)
	}
	ss := wal.cm.savedSettings()
	err = persist.Save(settingsMetadata, ss, wal.fileSettingsTmp)
	if err != nil {
		build.ExtendErr("unable to write to settings temp file", err)
	}

	wal.loadComplete = true
	return nil
}
