package contractmanager

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

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
		ErroredStorageFolderAdditions    []string
		StorageFolderAdditions           []*storageFolder

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
		cm *ContractManager
		loadComplete bool
		mu sync.Mutex
	}
)

// readWALMetadata reads WAL metadata from the input file, returning an error
// if the result is unexpected.
func readWALMetadata(f *os.File, decoder *json.Decoder) error {
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
	_, err = wal.file.Write(changeBytes)
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
	for _, sfa := range uc.StorageFolderAdditions {
		for i := uint64(0); i < wal.cm.dependencies.atLeastOne(); i++ {
			wal.commitAddStorageFolder(sfa)
		}
	}
}

// commit will take all of the changes that have been added to the WAL and
// atomically commit the WAL to disk, then apply the actions in the WAL to the
// state. commit will do lots of syncing disk I/O, and so can take a while,
// especially if there are a large number of actions queued up.
//
// A bool is returned indicating whether or not the commit was successful.
// False does not indiciate an error, it can also indicate that there was
// nothing to do.
//
// commit should only be called from threadedSyncLoop.
func (wal *writeAheadLog) commit() {
	// If there are no changes, do nothing.
	if len(wal.uncommittedChanges) == 0 {
		return
	}

	// Sync all open, non-WAL files on the host.
	//
	// The syncing is done in parallel to ensure that the total amount of time
	// consumed here is approximately equivalent to the cost of Syncing the
	// slowest file, instead of waiting to sync some files until other files
	// have finished syncing.
	//
	// On the previous commit, a bunch of files were given operations to
	// perform, but the operations were not synced. They have now had a second
	// or so to do buffer + write, which means a Sync call here should take
	// significantly less time than it would have if Sync had been called right
	// away. This is important to performance, as Sync must be called under
	// lock.
	var wg sync.WaitGroup
	// Queue a thread to sync + copy-on-write the settings file.
	wg.Add(1)
	go func() {
		tmpFilename := filepath.Join(wal.cm.persistDir, settingsFileTmp)
		filename := filepath.Join(wal.cm.persistDir, settingsFile)
		// Sync the settings file.
		err := wal.fileSettingsTmp.Sync()
		if err != nil {
			wal.cm.log.Critical("ERROR: unable to sync the contract manager settings:", err)
			panic("unable to sync contract manager settings, crashing to avoid data corruption")
		}
		err = wal.fileSettingsTmp.Close()
		if err != nil {
			wal.cm.log.Println("unable to close the temporary contract manager settings file:", err)
		}
		// COW the settings file.
		err = os.Rename(tmpFilename, filename)
		if err != nil {
			wal.cm.log.Critical("ERROR: unable to atomically copy the contract manager settings:", err)
			panic("unable to atomically copy contract manager settings, crashing to avoid data corruption")
		}
		// Remove the temporary file as the contents are no longer relevant.
		// Another will be created shortly.
		err = os.Remove(tmpFilename)
		if err != nil {
			wal.cm.log.Println("ERROR: unable to remove temporary settings file:", err)
		}
		wg.Done()
	}()
	// Queue threads to sync all of the storage folders.
	for _, sf := range cm.storageFolders {
		wg.Add(1)
		go func() {
			err := sf.file.Sync()
			if err != nil {
				wal.cm.log.Critical("ERROR: unable to sync a storage folder:", err)
				panic("unable to sync a storage folder, creashing to avoid data corruption")
			}
			wg.Done()
		}()
	}
	// Sync the temp WAL file, but DO NOT perform the copy-on-write - the COW
	// must happen AFTER all other files have successfully Sync'd.
	wg.Add(1)
	go func() {
		err := wal.fileWALTmp.Sync()
		if err != nil {
			wal.cm.log.Critical("Unable to sync the write-ahead-log:", err)
			panic("unable to sync the write-ahead-log, crashing to avoid data corrution")
		}
		err = wal.fileWALTmp.Close()
		if err != nil {
			// Log that the host is having trouble saving the uncommitted changes.
			// Crash if the list of uncommitted changes has grown very large.
			wal.cm.log.Println("ERROR: could not close temporary write-ahead-log in contract manager:", err)
		}
		wg.Done()
	}()
	wg.Wait()

	// The WAL file writes to a temporary file, so that there is a guarantee
	// that the actual WAL file will not ever be corrupted. Now that the tmp
	// file is closed and synced, rename the tmp file. Renaming is an atomic
	// action, meaning that the replacement to the WAL file is guaranteed not
	// to be corrupt.
	//
	// Note that this rename MUST occur AFTER all of the Sync calls above have
	// returned.
	walTmpName := filepath.Join(wal.cm.persistDir, walFileTmp)
	walFileName := filepath.Join(wal.cm.persistDir, walFile)
	err = os.Rename(walTmpName, walFileName)
	if err != nil {
		// Log that the host is having trouble saving the uncommitted changes.
		// Crash if the list of uncommitted changes has grown very large.
		wal.cm.log.Critical("ERROR: could not rename temporary write-ahead-log in contract manager:", err)
		panic("unable to copy-on-write the WAL temporary file, crashing to prevent data corruption")
	}
	// Remove the temporary WAL file as it has been committed. A new temporary
	// log file will be created shortly.
	err = os.Remove(walTmpName)
	if err != nil {
		wal.cm.log.Println("ERROR: unable to remove temporary write-ahead-log after successful commit-to-log:", err)
	}
	// Any module waiting for a synchronization has now had the changes
	// permanently committed, as of the WAL COW operation. Signal that syncing
	// is complete.
	close(wal.syncChan)
	wal.syncChan = make(chan struct{})

	// Take all of the changes that have been queued in the WAL and apply them
	// to the state. This will cause a lot of writing to the open files. These
	// writes will be Sync'd at the start of the next call to commit().
	for _, uc := range wal.uncommittedChanges {
		wal.applyChange(uc)
	}

	// Use helper functions to extract all remaining unfinished long-running
	// jobs from the WAL. Save these for later, when the WAL tmp file gets
	// reopened (the remaining unfinished jobs need to be written to the tmp
	// file to preserve their presence as understood by the WAL state).
	unfinishedAdditions := wal.findUnfinishedStorageFolderAdditions(wal.uncommittedChanges)
	// Set the list of uncommitted changes to nil, as everything has been
	// applied and/or saved to be appended later.
	wal.uncommittedChanges = nil

	// In parallel, open up the WAL and write to it, and open up the settings
	// file and write to it.
	wg.Add(1)
	go func() {
		// Begin writing to the settings file, but do not Sync. Syncing here
		// can cause the function to block for a long time while holding the
		// WAL lock. Instead, sync on the next iteration when this function has
		// had over a second to commit the data to disk; the sync should be
		// much faster.
		var err error
		wal.fileSettingsTmp, err = os.Create(filepath.Join(wal.cm.persistDir, settingsFileTmp)
		if err != nil {
			wal.cm.log.Critical("Unable to open temporary settings file for writing:", err)
			panic("unable to open temporary settings file for writing, crashing to prevent data corruption")
		}
		ss := wal.cm.savedSettings()
		err = persist.Save(settingsMetadata, ss, wal.fileSettingsTmp)
		if err != nil {
			wal.cm.log.Critical("writing to settings tmp file has failed:", err)
			panic("unable to write to temporary settings file, crashing to avoid data corruption")
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		// Recreate the wal file so that it can receive new updates.
		wal.fileWALTmp, err = os.Create(walTmpName)
		if err != nil {
			wal.cm.log.Critical("ERROR: unable to create write-ahead-log:", err)
			panic("unable to create write-ahead-log in contract manager, crashing to avoid data loss")
		}
		// Write the metadata into the WAL.
		err := writeWALMetadata(wal.file)
		if err != nil {
			wal.cm.log.Critical("Unable to properly initialize WAL file, crashing to prevent corruption:", err)
			panic("Unable to properly initialize WAL file, crashing to prevent corruption.")
		}
		// Append all of the remaining long running uncommitted changes to the WAL.
		err = wal.appendChange(stateChange{
			UnfinishedStorageFolderAdditions: unfinishedAdditions,
		})
		if err != nil {
			wal.cm.log.Println("ERROR: in-progress action lost due to disk failure:", err)
		}
		wg.Done()
	}
	wg.Wait()
}

// load will pull any changes from the uncommited WAL into memory, decoding
// them and doing any necessary preprocessing. In the most common case (any
// time the previous shutdown was clean), there will not be a WAL file.
func (wal *writeAheadLog) load() error {
	// Open up the WAL file.
	walFileName := filepath.Join(wal.cm.persistDir, walFile)
	file, err := os.Open(walFileName)
	if os.IsNotExist(err) {
		// There is no WAL, which most likely means that everything has
		// committed safely, and that there was a clean shutdown.
		return nil
	} else if err != nil {
		return build.ExtendErr("walFile was not opened successfully", err)
	}
	defer file.Close()

	// Read the WAL metadata to make sure that the version is correct.
	decoder := json.NewDecoder(file)
	err = readWALMetadata(file, decoder)
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
	var remainingChanges []stateChange
	remainingUSFAs, err := wal.cleanupUnfinishedStorageFolderAdditions(scs)
	if err != nil {
		return build.ExtendErr("error performing unfinished storage folder cleanup", err)
	}
	remainingChanges = append(remainingChanges, remainingUSFAs...)

	// Create the walTmpFile and write all of the remaining long running
	// changes to it.
	walTmpName := filepath.Join(wal.cm.persistDir, walFileTmp)
	wal.fileWALTmp, err = os.Create(walTmpName)
	if err != nil {
		return build.ExtendErr("unable to open WAL temporary file", err)
	}
	err = writeWALMetadata(wal.file)
	if err != nil {
		return build.ExtendErr("unable to write WAL metadata", err)
	}
	for _, rc := range remainingChanges {
		err = wal.appendChange(rc)
		if err != nil {
			return build.ExtendErr("could not re-add an uncommitted change to the WAL temporary file", err)
		}
	}

	// Open up the settings tmp file and write to it. This will finish
	// preparations and allow the sync loop to get started without hiccups.
	wal.fileSettingsTmp, err = os.Create(filepath.Join(cm.persistDir, settingsFileTmp))
	if err != nil {
		return build.ExtendErr("unable to prepare the settings temp file", err)
	}
	ss := wal.cm.savedSettings()
	err = persist.Save(settingsMetadata, ss, wal.fileSettingsTmp)
	if err != nil {
		build.ExtendErr("unable to write to settings temp file", err)
	}

	// Perform a commit() before returning to guarantee that the in-progress
	// changes have completed.
	wal.commit()

	// TODO: Run any sanity checks desired. For example, should use SWAR to
	// verify that the 'Usage', 'Sectors', and total capacity of each storage
	// folder add up.

	wal.loadComplete = true
	return nil
}

// spawnSyncLoop prepares and establishes the loop which will be running in the
// background to coordinate disk syncronizations. Disk syncing is done in a
// background loop to help with performance, and to allow multiple things to
// modify the WAL simultaneously.
func (wal *writeAheadLog) spawnSyncLoop() (err error) {
	// Create a signal so we know when the sync loop has stopped, which means
	// there will be no more open commits.
	threadsStopped := make(chan struct{})
	syncLoopStopped := make(chan struct{})
	wal.syncChan = make(chan struct{})
	go wal.threadedSyncLoop(threadsStopped, syncLoopStopped)
	wal.cm.tg.AfterStop(func() {
		// Close the threadsStopped channel to let the sync loop know that all
		// calls to tg.Add() in the contract manager have cleaned up.
		close(threadsStopped)

		// Because this is being called in an 'AfterStop' routine, all open
		// calls to the contract manager should have completed, and all open
		// threads should have closed. The last call to change the contract
		// manager should have completed, so the number of uncommitted changes
		// should be zero.
		<-syncLoopStopped // Wait for the sync loop to signal proper termination.

		// Close the dangling commit resources.
		err = wal.fileWALTmp.Close()
		if err != nil {
			wal.cm.log.Println("Error closing wal file during contract manager shutdown:", err)
		}
		err = os.Remove(filepath.Join(wal.cm.persistDir, walFileTmp))
		if err != nil {
			wal.cm.log.Println("Error removing temporary WAL during contract manager shutdown:", err)
		}
		err = os.Remove(filepath.Join(wal.cm.persistDir, walFile))
		if err != nil {
			wal.cm.log.Println("Error removing WAL during contract manager shutdown:", err)
		}
	})
	return nil
}

// threadedSyncLoop is a background thread that occasionally commits the WAL to
// the state as an ACID transaction. This process can be very slow, so
// transactions to the contract manager are batched automatically and
// occasionally committed together.
func (wal *writeAheadLog) threadedSyncLoop(threadsStopped chan struct{}, syncLoopStopped chan struct{}) {
	// Provide a place for the testing to disable the sync loop.
	if wal.cm.dependencies.disrupt("threadedSyncLoopStart") {
		close(syncLoopStopped)
		return
	}

	syncInterval := 1500 * time.Millisecond
	for {
		select {
		case <-threadsStopped:
			close(syncLoopStopped)
			return
		case <-time.After(syncInterval):
			// Commit all of the changes in the WAL to disk, and then apply the
			// changes. Measure how long it takes to apply the changes, and use
			// that to steer the amount of time that should be waited to the
			// next sync.
			start := time.Now()
			wal.mu.Lock()
			wal.commit()
			wal.mu.Unlock()
			syncTime := time.Since(start)
		}
	}
}
