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
		UnfinishedStorageFolderAdditions []*storageFolder
	}

	// writeAheadLog coordinates ACID transaction which update the state of the
	// contract manager. WAL should be the only entity that is accessing the
	// contract manager's mutex.
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
func (wal *writeAheadLog) commit() bool {
	// If there is nothing to do, do nothing.
	if len(wal.uncommittedChanges) == 0 {
		return false
	}

	// Sync the WAL file, so that there is a guarantee that all pending actions
	// are safe from power disruptions.
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

	// The WAL file writes to a temporary file, so that there is a guarantee
	// that the actual WAL file will not ever be corrupted. Now that the tmp
	// file is closed and synced, rename the tmp file. Renaming is an atomic
	// action, meaning that the replacement to the WAL file is guaranteed not
	// to be corrupt.
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
	// Remove the temporary WAL file as it's no longer useful.
	err = os.Remove(walTmpName)
	if err != nil {
		wal.cm.log.Println("ERROR: unable to remove temporary write-ahead-log after successful commit-to-log")
	}

	// Take all of the changes that have been queued in the WAL and apply them
	// to the state.
	//
	// All commits should be idempotent. To verify that the commits are
	// idempotent, in debug mode the commits are occasionally applied multiple
	// times. If they are idempotent, testing should be entirely unaffected.
	for _, uc := range wal.uncommittedChanges {
		for _, sfa := range uc.StorageFolderAdditions {
			// Rather than have some code that will randomly run the commit
			// multiple times if testing is active, a dependency is used that
			// tester's can hijack to guarantee that the action will be run
			// multiple times, or if needed can also guarantee that it'll be
			// run exactly once. In productions, atLeastOne() will always
			// return exactly 1.
			for i := uint64(0); i < wal.cm.dependencies.atLeastOne(); i++ {
				wal.commitAddStorageFolder(sfa)
			}
		}
	}

	// Use helper functions to extract all remaining unfinished long-running
	// jobs from the WAL. Save these for later, when the WAL tmp file gets
	// reopened (the remaining unfinished jobs need to be written to the tmp
	// file to preserve their presence as understood by the WAL state).
	unfinishedAdditions := wal.findUnfinishedStorageFolderAdditions(wal.uncommittedChanges)

	// Set the list of uncommitted changes to nil, as everything has been
	// applied and/or saved to be appended later.
	wal.uncommittedChanges = nil

	// Save the updated contract manager to disk, syncing to be certain that
	// all changes will be durable.
	err = wal.cm.saveSync()
	if err != nil {
		// Log an error and abort the commitment. Commitments are not currently
		// reversible, which means that the only way to protect the contract
		// manager from corruption at this point is to crash. If we crash,
		// there will be no corruption as the save process is atomic.
		wal.cm.log.Critical("Could not save the contract manager - crashing to prevent corruption.")
		panic("Could not save the contract manager - crashing to prevent corruption.")
	}
	// Recreate the wal file so that it can receive new updates.
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
	// Write the WAL metadata to the file.
	err = writeWALMetadata(wal.file)
	if err != nil {
		wal.cm.log.Critical("Unable to properly initialize WAL file, crashing to prevent corruption.")
		panic("Unable to properly initialize WAL file, crashing to prevent corruption.")
	}
	// Append all of the remaining long running uncommitted changes to the WAL.
	err = wal.appendChange(stateChange{
		UnfinishedStorageFolderAdditions: unfinishedAdditions,
	})
	if err != nil {
		wal.cm.log.Println("ERROR: in-progress action lost due to disk failure")
	}
	// Remove the WAL file, as all changes have been applied successfully.
	err = os.Remove(walFileName)
	if err != nil {
		wal.cm.log.Println("trouble removing WAL - no risk of corruption, but the file is not needed anymore either")
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
	var sc stateChange
	for err == nil {
		err = decoder.Decode(&sc)
		if err == nil {
			// The uncommitted changes are loaded into memory using a simple
			// append, because the tmp WAL file has not been created yet, and
			// will not be created until the sync loop is spawned. The sync
			// loop spawner will make sure that the uncommitted changes are
			// written to the tmp WAL file.
			wal.uncommittedChanges = append(wal.uncommittedChanges, sc)
		}
	}
	if err != io.EOF {
		return build.ExtendErr("error loading WAL json", err)
	}

	// Do any cleanup regarding long-running unfinished tasks.
	wal.cleanupUnfinishedStorageFolderAdditions()
	return nil
}

// spawnSyncLoop prepares and establishes the loop which will be running in the
// background to coordinate disk syncronizations. Disk syncing is done in a
// background loop to help with performance, and to allow multiple things to
// modify the WAL simultaneously.
func (wal *writeAheadLog) spawnSyncLoop() (err error) {
	// Create the file resource that gets used when committing. Then establish
	// the AfterStop call that will close the file resource.
	walTmpName := filepath.Join(wal.cm.persistDir, walFileTmp)
	wal.file, err = os.Create(walTmpName)
	if err != nil {
		return build.ExtendErr("unable to open WAL temporary file", err)
	}
	err = writeWALMetadata(wal.file)
	if err != nil {
		return build.ExtendErr("unable to write WAL metadata", err)
	}

	// During loading, a bunch of changes may have been pulled into the WAL's
	// memory, but were not written to disk because the temporary WAL file had
	// not been created yet.
	ucs := wal.uncommittedChanges
	wal.uncommittedChanges = nil
	for _, uc := range ucs {
		err = wal.appendChange(uc)
		if err != nil {
			return build.ExtendErr("could not re-add an uncommitted change to the WAL temporary file", err)
		}
	}

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
		err = wal.file.Close()
		if err != nil {
			wal.cm.log.Println("Error closing walfile during contract manager shutdown:", err)
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

	syncInterval := time.Millisecond * 100
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
