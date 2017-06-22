package contractmanager

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/fastrand"
)

type (
	// walHeader is the header of the wal file
	//
	// LengthMD defines the changeLength (in bytes) of the metadata section right after
	// the walHeader
	//
	// Revision defines the current revision of the WAL. If an older revision than the one
	// in the header is encountered during a recovery it stops reading the WAL
	walHeader struct {
		LengthMD int64
		Revision uint64
	}

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

		// Revision describes the revision the stateChange belongs to. After the WAL is full
		// it will process the changes, restart from the beginning and increase the revision
		// to invalidate old stateChanges should they still be readable.
		Revision uint64
	}

	// stateChangePrefix defines a short prefix of constant size that is written to the WAL before each
	// stateChange. I contains a checksum and changeLength of the encoded stateChange
	stateChangePrefix struct {
		ChangeLength int64
		Checksum     crypto.Hash
	}

	// writeAheadLog coordinates ACID transactions which update the state of
	// the contract manager. Consistency on a field is only guaranteed by
	// looking it up through the WAL, and is not guaranteed by direct access.
	writeAheadLog struct {
		// TODO change this before merging
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
		fileSettingsTmp    file
		fileWal            file
		header             walHeader
		changeOffset       int64
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
func readWALMetadata(f file, header walHeader) error {
	// Check if the metadata of the file has the same length as the current version
	if ml, err := metadataLength(); (err != nil) || (header.LengthMD != ml) {
		return errors.New("WAL metadata header does not have the same length as the one in the WAL." +
			"The file might use an older version or the header was corrupted")
	}

	// Read the bytes of the metadata
	byteMD := make([]byte, header.LengthMD)
	_, err := f.ReadAt(byteMD, headerLength())
	if err != nil {
		return build.ExtendErr("error reading WAL metadata", err)
	}

	// Decode the metadata
	decoder := json.NewDecoder(bytes.NewBuffer(byteMD))
	var md persist.Metadata
	err = decoder.Decode(&md)
	if err != nil {
		return build.ExtendErr("error decoding WAL metadata", err)
	}

	// Check if correct header was found
	if md.Header != walMetadata.Header {
		return errors.New("WAL metadata header does not match header found in WAL file")
	}
	if md.Version != walMetadata.Version {
		return errors.New("WAL metadata version does not match version found in WAL file")
	}
	return nil
}

// writeWALMetadata writes WAL metadata to the input file.
func writeWALMetadata(f file) (int64, error) {
	mdBytes, err := json.MarshalIndent(walMetadata, "", "\t")
	if err != nil {
		return 0, build.ExtendErr("could not marshal WAL metadata", err)
	}

	_, err = f.WriteAt(mdBytes, headerLength())
	if err != nil {
		return 0, build.ExtendErr("unable to write WAL metadata", err)
	}
	return int64(len(mdBytes)), nil
}

// headerLength is a helper function that returns the changeLength of the binary encoded wal header
func headerLength() int64 {
	return int64(binary.Size(walHeader{}))
}

// metadataLength is a helper function that returns the length of the binary encoded metadata
func metadataLength() (int64, error) {
	mdBytes, err := json.MarshalIndent(walMetadata, "", "\t")
	if err != nil {
		return 0, build.ExtendErr("could not marshal WAL metadata", err)
	}
	return int64(len(mdBytes)), nil
}

// prefixLength is a helper function that returns the length of a binary encoded stateChange prefix
func prefixLength() int64 {
	return int64(binary.Size(stateChangePrefix{}))
}

// readWALHeader reads the WAL header from the input file
func readWALHeader(f file, header *walHeader) error {
	bytesHeader := make([]byte, headerLength())

	_, err := f.ReadAt(bytesHeader, 0)
	if err != nil {
		return build.ExtendErr("could not read WAL header", err)
	}
	buffer := bytes.NewBuffer(bytesHeader)

	err = binary.Read(buffer, binary.LittleEndian, header)
	if err != nil {
		return build.ExtendErr("could not decode WAL header", err)
	}
	return nil
}

// writeWALHeader writes the WAL header to the input file
func writeWALHeader(f file, header walHeader) error {
	buffer := new(bytes.Buffer)

	err := binary.Write(buffer, binary.LittleEndian, &header)
	if err != nil {
		return build.ExtendErr("could not marshal WAL header", err)
	}
	_, err = f.WriteAt(buffer.Bytes(), 0)
	if err != nil {
		return build.ExtendErr("could not write WAL header", err)
	}
	return nil
}

// readStateChange reads a stateChange at a specific offset in the file and
// returns the changeLength of the stateChange read
func (wal *writeAheadLog) readStateChange(offset int64, sc *stateChange) (int64, error) {
	// Read the prefix
	bytesPrefix := make([]byte, prefixLength())

	_, err := wal.fileWal.ReadAt(bytesPrefix, offset)
	if err != nil {
		return 0, build.ExtendErr("could not read stateChange prefix", err)
	}

	// Decode the prefix
	buffer := bytes.NewBuffer(bytesPrefix)
	var prefix stateChangePrefix

	err = binary.Read(buffer, binary.LittleEndian, &prefix)
	if err != nil {
		return 0, build.ExtendErr("could not decode stateChange prefix", err)
	}

	// Don't trust changeLength. It might be corrupted
	if prefix.ChangeLength > maxWalSize-offset {
		return 0, build.ExtendErr("changeLength was too large. Probably due to corruption", err)
	}

	// Read the state change
	bytesChange := make([]byte, prefix.ChangeLength)

	_, err = wal.fileWal.ReadAt(bytesChange, offset+prefixLength())
	if err != nil {
		return 0, build.ExtendErr("could not read stateChange", err)
	}

	// Check the stateChange for corruption before decoding it
	checksum := crypto.HashBytes(bytesChange)
	if bytes.Compare(prefix.Checksum[:], checksum[:]) != 0 {
		return 0, errors.New("stateChange is corrupted")
	}

	// Decode the state change
	buffer = bytes.NewBuffer(bytesChange)
	decoder := json.NewDecoder(buffer)

	err = decoder.Decode(sc)
	if err != nil {
		return 0, build.ExtendErr("could not decode stateChange", err)
	}

	// Check if the revision of the stateChange matches the header
	if sc.Revision != wal.header.Revision {
		return 0, errors.New("stateChange doesn't match the current header's revision")
	}

	return prefix.ChangeLength, nil
}

// appendChange will add a change to the WAL, writing the details of the change
// to the WAL file but not syncing - syncing is orchestrated by the sync loop.
//
// The WAL is append only, which means that changes can only be revoked by
// appending an error. This is common for long running operations like adding a
// storage folder.
func (wal *writeAheadLog) appendChange(sc stateChange) {
	// set the revision to which the stateChange is added
	sc.Revision = wal.header.Revision

	// Marshal the change and then write the change to the WAL file. Syncing
	// happens in the sync loop.
	changeBytes, err := json.MarshalIndent(sc, "", "\t")
	changeLength := int64(len(changeBytes))
	if err != nil {
		wal.cm.log.Severe("Unable to marshal state change:", err)
		panic("unable to append a change to the WAL, crashing to prevent corruption")
	}

	// Create and encode a new prefix
	prefix := stateChangePrefix{
		ChangeLength: changeLength,
		Checksum:     crypto.HashBytes(changeBytes),
	}
	var buffer bytes.Buffer
	err = binary.Write(&buffer, binary.LittleEndian, prefix)
	if err != nil {
		wal.cm.log.Severe("Unable to encode state change prefix:", err)
		panic("unable to append a change to the WAL, crashing to prevent corruption")
	}

	// Write the prefix to the file followed by the state change
	_, err = wal.fileWal.WriteAt(buffer.Bytes(), wal.changeOffset)
	if err != nil {
		wal.cm.log.Severe("Unable to write state change prefix to WAL:", err)
		panic("unable to append a change to the WAL, crashing to prevent corruption")
	}

	_, err = wal.fileWal.WriteAt(changeBytes, wal.changeOffset+prefixLength())
	if err != nil {
		wal.cm.log.Severe("Unable to write state change to WAL:", err)
		panic("unable to append a change to the WAL, crashing to prevent corruption")
	}

	// Simulate a corruption of the stateChange when the StorageFolderAddition is appended.
	if wal.cm.dependencies.disrupt("walCorruptedChange") && len(sc.StorageFolderAdditions) > 0 {
		wal.fileWal.WriteAt(fastrand.Bytes(len(changeBytes)), wal.changeOffset+prefixLength())
	}

	// Update the offset
	wal.changeOffset += prefixLength() + changeLength

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
		for i := uint64(0); i < wal.cm.dependencies.atLeastOne(); i++ {
			wal.commitAddStorageFolder(sfa)
		}
	}
	for _, sfe := range sc.StorageFolderExtensions {
		for i := uint64(0); i < wal.cm.dependencies.atLeastOne(); i++ {
			wal.commitStorageFolderExtension(sfe)
		}
	}
	for _, sfr := range sc.StorageFolderReductions {
		for i := uint64(0); i < wal.cm.dependencies.atLeastOne(); i++ {
			wal.commitStorageFolderReduction(sfr)
		}
	}
	for _, sfr := range sc.StorageFolderRemovals {
		for i := uint64(0); i < wal.cm.dependencies.atLeastOne(); i++ {
			wal.commitStorageFolderRemoval(sfr)
		}
	}
	for _, su := range sc.SectorUpdates {
		for i := uint64(0); i < wal.cm.dependencies.atLeastOne(); i++ {
			wal.commitUpdateSector(su)
		}
	}
}

// createWAL will open up the WAL file.
func (wal *writeAheadLog) createWAL() {
	var err error
	walName := filepath.Join(wal.cm.persistDir, walFile)
	wal.fileWal, err = wal.cm.dependencies.createFile(walName)
	if err != nil {
		wal.cm.log.Severe("Unable to create WAL file:", err)
		panic("unable to create WAL file, crashing to avoid corruption")
	}

	lengthMD, err := writeWALMetadata(wal.fileWal)
	if err != nil {
		wal.cm.log.Severe("Unable to write WAL metadata:", err)
		panic("unable to create WAL file, crashing to prevent corruption")
	}

	header := walHeader{
		LengthMD: lengthMD,
	}

	err = writeWALHeader(wal.fileWal, header)
	if err != nil {
		wal.cm.log.Severe("Unable to write WAL header:", err)
		panic("unable to write WAL header, crashing to avoid corruption")
	}
	wal.header = header
	wal.changeOffset = headerLength() + lengthMD
}

// recoverWAL will read a previous WAL and re-commit all of the changes inside,
// restoring the program to consistency after an unclean shutdown. The tmp WAL
// file needs to be open before this function is called.
func (wal *writeAheadLog) recoverWAL() error {
	// Read the WAL header and save it
	var header walHeader
	err := readWALHeader(wal.fileWal, &header)
	if err != nil {
		wal.cm.log.Println("ERROR: error while reading WAL header:", err)
		return err
	}
	wal.header = header

	// Read the WAL metadata to make sure that the version is correct.
	err = readWALMetadata(wal.fileWal, wal.header)
	if err != nil {
		wal.cm.log.Println("ERROR: error while reading WAL metadata:", err)
		return build.ExtendErr("walFile metadata mismatch", err)
	}

	// Read changes from the WAL one at a time and load them back into memory.
	// A full list of changes is kept so that modifications to long running
	// changes can be parsed properly.
	var sc stateChange
	var scs []stateChange
	offset := headerLength() + header.LengthMD
	for offset < maxWalSize {
		changeLength, err := wal.readStateChange(offset, &sc)
		if err != nil {
			break
		}
		// The uncommitted changes are loaded into memory using a simple
		// append, because the WAL file is still being read. The sync
		// loop spawner will make sure that the uncommitted changes are
		// written to the new WAL file.
		wal.commitChange(sc)
		scs = append(scs, sc)
		offset += prefixLength() + changeLength
	}
	//if err != nil {
	//	wal.cm.log.Println("ERROR: could not load WAL json:", err)
	//	return build.ExtendErr("error loading WAL json", err)
	//}

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
	// Try opening the WAL file.
	walFileName := filepath.Join(wal.cm.persistDir, walFile)
	var err error
	wal.fileWal, err = wal.cm.dependencies.openFile(walFileName, os.O_RDWR, 0600)
	if err == nil {
		// err == nil indicates that there is a WAL file, which means that the
		// previous shutdown was not clean. Re-commit the changes in the WAL to
		// bring the program back to consistency.
		wal.cm.log.Println("WARN: WAL file detected, performing recovery after unclean shutdown.")
		err = wal.recoverWAL()
		if err != nil {
			wal.fileWal.Close()
			return build.ExtendErr("failed to recover WAL", err)
		}
		err = wal.fileWal.Close()
		if err != nil {
			return build.ExtendErr("error closing WAL after performing a recovery", err)
		}

	} else if !os.IsNotExist(err) {
		return build.ExtendErr("walFile was not opened successfully", err)
	}
	// Create/Reset the WAL
	wal.createWAL()

	// err == os.IsNotExist, suggesting a successful, clean shutdown. A new WAL is created but no additional action
	// is taken.

	// Create the tmp settings file and initialize the first write to it. This
	// is necessary before kicking off the sync loop.
	wal.fileSettingsTmp, err = wal.cm.dependencies.createFile(filepath.Join(wal.cm.persistDir, settingsFileTmp))
	if err != nil {
		return build.ExtendErr("unable to prepare the settings temp file", err)
	}
	wal.cm.tg.AfterStop(func() {
		wal.mu.Lock()
		defer wal.mu.Unlock()
		err := wal.fileSettingsTmp.Close()
		if err != nil {
			wal.cm.log.Println("ERROR: unable to close settings temporary file")
			return
		}
		err = wal.cm.dependencies.removeFile(filepath.Join(wal.cm.persistDir, settingsFileTmp))
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
