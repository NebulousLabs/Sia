package renter

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/siafile"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/errors"
)

var (
	// ErrEmptyFilename is an error when filename is empty
	ErrEmptyFilename = errors.New("filename must be a nonempty string")
	// ErrPathOverload is an error when a file already exists at that location
	ErrPathOverload = errors.New("a file already exists at that location")
	// ErrUnknownPath is an error when a file cannot be found with the given path
	ErrUnknownPath = errors.New("no file known with that path")
)

// A file is a single file that has been uploaded to the network. Files are
// split into equal-length chunks, which are then erasure-coded into pieces.
// Each piece is separately encrypted, using a key derived from the file's
// master key. The pieces are uploaded to hosts in groups, such that one file
// contract covers many pieces.
type file struct {
	name        string
	size        uint64 // Static - can be accessed without lock.
	contracts   map[types.FileContractID]fileContract
	masterKey   crypto.TwofishKey    // Static - can be accessed without lock.
	erasureCode modules.ErasureCoder // Static - can be accessed without lock.
	pieceSize   uint64               // Static - can be accessed without lock.
	mode        uint32               // actually an os.FileMode
	deleted     bool                 // indicates if the file has been deleted.

	staticUID string // A UID assigned to the file when it gets created.

	mu sync.RWMutex
}

// A fileContract is a contract covering an arbitrary number of file pieces.
// Chunk/Piece metadata is used to split the raw contract data appropriately.
type fileContract struct {
	ID     types.FileContractID
	IP     modules.NetAddress
	Pieces []pieceData

	WindowStart types.BlockHeight
}

// pieceData contains the metadata necessary to request a piece from a
// fetcher.
//
// TODO: Add an 'Unavailable' flag that can be set if the host loses the piece.
// Some TODOs exist in 'repair.go' related to this field.
type pieceData struct {
	Chunk      uint64      // which chunk the piece belongs to
	Piece      uint64      // the index of the piece in the chunk
	MerkleRoot crypto.Hash // the Merkle root of the piece
}

// deriveKey derives the key used to encrypt and decrypt a specific file piece.
func deriveKey(masterKey crypto.TwofishKey, chunkIndex, pieceIndex uint64) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, chunkIndex, pieceIndex))
}

// DeleteFile removes a file entry from the renter and deletes its data from
// the hosts it is stored on.
//
// TODO: The data is not cleared from any contracts where the host is not
// immediately online.
func (r *Renter) DeleteFile(nickname string) error {
	lockID := r.mu.Lock()
	f, exists := r.files[nickname]
	if !exists {
		r.mu.Unlock(lockID)
		return ErrUnknownPath
	}
	delete(r.files, nickname)
	delete(r.persist.Tracking, nickname)

	r.saveSync()
	r.mu.Unlock(lockID)

	// TODO: delete the sectors of the file as well.
	return errors.AddContext(f.Delete(), "failed to delete file")
}

// FileList returns all of the files that the renter has.
func (r *Renter) FileList() []modules.FileInfo {
	// Get all the files holding the readlock.
	lockID := r.mu.RLock()
	files := make([]*siafile.SiaFile, 0, len(r.files))
	for _, file := range r.files {
		files = append(files, file)
	}
	r.mu.RUnlock(lockID)

	// Save host keys in map. We can't do that under the same lock since we
	// need to call a public method on the file.
	pks := make(map[string]types.SiaPublicKey)
	for _, f := range r.files {
		for _, pk := range f.HostPublicKeys() {
			pks[string(pk.Key)] = pk
		}
	}

	// Build 2 maps that map every pubkey to its offline and goodForRenew
	// status.
	goodForRenew := make(map[string]bool)
	offline := make(map[string]bool)
	contracts := make(map[string]modules.RenterContract)
	for _, pk := range pks {
		contract, ok := r.hostContractor.ContractByPublicKey(pk)
		if !ok {
			continue
		}
		goodForRenew[string(pk.Key)] = ok && contract.Utility.GoodForRenew
		offline[string(pk.Key)] = r.hostContractor.IsOffline(pk)
		contracts[string(pk.Key)] = contract
	}

	// Build the list of FileInfos.
	fileList := []modules.FileInfo{}
	for _, f := range files {
		var localPath string
		siaPath := f.SiaPath()
		lockID := r.mu.RLock()
		tf, exists := r.persist.Tracking[siaPath]
		r.mu.RUnlock(lockID)
		if exists {
			localPath = tf.RepairPath
		}

		fileList = append(fileList, modules.FileInfo{
			SiaPath:        f.SiaPath(),
			LocalPath:      localPath,
			Filesize:       f.Size(),
			Renewing:       true,
			Available:      f.Available(offline),
			Redundancy:     f.Redundancy(offline, goodForRenew),
			UploadedBytes:  f.UploadedBytes(),
			UploadProgress: f.UploadProgress(),
			Expiration:     f.Expiration(contracts),
		})
	}
	return fileList
}

// File returns file from siaPath queried by user.
// Update based on FileList
func (r *Renter) File(siaPath string) (modules.FileInfo, error) {
	var fileInfo modules.FileInfo

	// Get the file and its contracts
	lockID := r.mu.RLock()
	file, exists := r.files[siaPath]
	r.mu.RUnlock(lockID)
	if !exists {
		return fileInfo, ErrUnknownPath
	}
	pks := file.HostPublicKeys()

	// Build 2 maps that map every contract id to its offline and goodForRenew
	// status.
	goodForRenew := make(map[string]bool)
	offline := make(map[string]bool)
	contracts := make(map[string]modules.RenterContract)
	for _, pk := range pks {
		contract, ok := r.hostContractor.ContractByPublicKey(pk)
		if !ok {
			continue
		}
		goodForRenew[string(pk.Key)] = ok && contract.Utility.GoodForRenew
		offline[string(pk.Key)] = r.hostContractor.IsOffline(pk)
		contracts[string(pk.Key)] = contract
	}

	// Build the FileInfo
	renewing := true
	var localPath string
	tf, exists := r.persist.Tracking[file.SiaPath()]
	if exists {
		localPath = tf.RepairPath
	}
	fileInfo = modules.FileInfo{
		SiaPath:        file.SiaPath(),
		LocalPath:      localPath,
		Filesize:       file.Size(),
		Renewing:       renewing,
		Available:      file.Available(offline),
		Redundancy:     file.Redundancy(offline, goodForRenew),
		UploadedBytes:  file.UploadedBytes(),
		UploadProgress: file.UploadProgress(),
		Expiration:     file.Expiration(contracts),
	}

	return fileInfo, nil
}

// RenameFile takes an existing file and changes the nickname. The original
// file must exist, and there must not be any file that already has the
// replacement nickname.
func (r *Renter) RenameFile(currentName, newName string) error {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	err := validateSiapath(newName)
	if err != nil {
		return err
	}

	// Check that currentName exists and newName doesn't.
	file, exists := r.files[currentName]
	if !exists {
		return ErrUnknownPath
	}
	_, exists = r.files[newName]
	if exists {
		return ErrPathOverload
	}

	// Modify the file and save it to disk.
	file.Rename(newName) // TODO: violation of locking convention
	err = r.saveFile(file)
	if err != nil {
		return err
	}

	// Update the entries in the renter.
	delete(r.files, currentName)
	r.files[newName] = file
	if t, ok := r.persist.Tracking[currentName]; ok {
		delete(r.persist.Tracking, currentName)
		r.persist.Tracking[newName] = t
	}
	err = r.saveSync()
	if err != nil {
		return err
	}

	// Delete the old .sia file.
	oldPath := filepath.Join(r.persistDir, currentName+ShareExtension)
	return os.RemoveAll(oldPath)
}

// fileToSiaFile converts a legacy file to a SiaFile. Fields that can't be
// populated using the legacy file remain blank.
func fileToSiaFile(f *file) *siafile.SiaFile {
	panic("not implemented yet")
}

// siaFileToFile converts a SiaFile to a legacy file. Fields that don't exist
// in the legacy file will get lost and therefore not persisted.
func siaFileToFile(sf *siafile.SiaFile) *file {
	panic("not implemented yet")
}
