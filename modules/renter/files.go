package renter

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
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

// staticChunkSize returns the size of one chunk.
func (f *file) staticChunkSize() uint64 {
	return f.pieceSize * uint64(f.erasureCode.MinPieces())
}

// numChunks returns the number of chunks that f was split into.
func (f *file) numChunks() uint64 {
	// empty files still need at least one chunk
	if f.size == 0 {
		return 1
	}
	n := f.size / f.staticChunkSize()
	// last chunk will be padded, unless chunkSize divides file evenly.
	if f.size%f.staticChunkSize() != 0 {
		n++
	}
	return n
}

// available indicates whether the file is ready to be downloaded.
func (f *file) available(offline map[types.FileContractID]bool) bool {
	chunkPieces := make([]int, f.numChunks())
	for _, fc := range f.contracts {
		if offline[fc.ID] {
			continue
		}
		for _, p := range fc.Pieces {
			chunkPieces[p.Chunk]++
		}
	}
	for _, n := range chunkPieces {
		if n < f.erasureCode.MinPieces() {
			return false
		}
	}
	return true
}

// uploadedBytes indicates how many bytes of the file have been uploaded via
// current file contracts. Note that this includes padding and redundancy, so
// uploadedBytes can return a value much larger than the file's original filesize.
func (f *file) uploadedBytes() uint64 {
	var uploaded uint64
	for _, fc := range f.contracts {
		// Note: we need to multiply by SectorSize here instead of
		// f.pieceSize because the actual bytes uploaded include overhead
		// from TwoFish encryption
		uploaded += uint64(len(fc.Pieces)) * modules.SectorSize
	}
	return uploaded
}

// uploadProgress indicates what percentage of the file (plus redundancy) has
// been uploaded. Note that a file may be Available long before UploadProgress
// reaches 100%, and UploadProgress may report a value greater than 100%.
func (f *file) uploadProgress() float64 {
	uploaded := f.uploadedBytes()
	desired := modules.SectorSize * uint64(f.erasureCode.NumPieces()) * f.numChunks()

	return math.Min(100*(float64(uploaded)/float64(desired)), 100)
}

// redundancy returns the redundancy of the least redundant chunk. A file
// becomes available when this redundancy is >= 1. Assumes that every piece is
// unique within a file contract. -1 is returned if the file has size 0. It
// takes one argument, a map of offline contracts for this file.
func (f *file) redundancy(offlineMap map[types.FileContractID]bool, goodForRenewMap map[types.FileContractID]bool) float64 {
	if f.size == 0 {
		return -1
	}
	piecesPerChunk := make([]int, f.numChunks())
	piecesPerChunkNoRenew := make([]int, f.numChunks())
	// If the file has non-0 size then the number of chunks should also be
	// non-0. Therefore the f.size == 0 conditional block above must appear
	// before this check.
	if len(piecesPerChunk) == 0 {
		build.Critical("cannot get redundancy of a file with 0 chunks")
		return -1
	}
	pieceMap := make(map[string]struct{})
	for _, fc := range f.contracts {
		offline := offlineMap[fc.ID]
		goodForRenew := goodForRenewMap[fc.ID]

		// do not count pieces from the contract if the contract is offline
		if offline {
			continue
		}
		for _, p := range fc.Pieces {
			pieceKey := fmt.Sprintf("%v/%v", p.Chunk, p.Piece)
			if _, redundant := pieceMap[pieceKey]; redundant {
				continue
			}
			pieceMap[pieceKey] = struct{}{}
			if goodForRenew {
				piecesPerChunk[p.Chunk]++
			}
			piecesPerChunkNoRenew[p.Chunk]++
		}
	}
	// Find the chunk with the least finished pieces counting only pieces of
	// contracts that are goodForRenew.
	minPieces := piecesPerChunk[0]
	for _, numPieces := range piecesPerChunk {
		if numPieces < minPieces {
			minPieces = numPieces
		}
	}
	// Find the chunk with the least finished pieces including pieces from
	// contracts that are not good for renewal.
	minPiecesNoRenew := piecesPerChunkNoRenew[0]
	for _, numPieces := range piecesPerChunkNoRenew {
		if numPieces < minPiecesNoRenew {
			minPiecesNoRenew = numPieces
		}
	}
	// If the redundancy is smaller than 1x we return the redundancy that
	// includes contracts that are not good for renewal. The reason for this is
	// a better user experience. If the renter operates correctly, redundancy
	// should never go above numPieces / minPieces and redundancyNoRenew should
	// never go below 1.
	redundancy := float64(minPieces) / float64(f.erasureCode.MinPieces())
	redundancyNoRenew := float64(minPiecesNoRenew) / float64(f.erasureCode.MinPieces())
	if redundancy < 1 {
		return redundancyNoRenew
	}
	return redundancy
}

// expiration returns the lowest height at which any of the file's contracts
// will expire.
func (f *file) expiration() types.BlockHeight {
	if len(f.contracts) == 0 {
		return 0
	}
	lowest := ^types.BlockHeight(0)
	for _, fc := range f.contracts {
		if fc.WindowStart < lowest {
			lowest = fc.WindowStart
		}
	}
	return lowest
}

// newFile creates a new file object.
func newFile(name string, code modules.ErasureCoder, pieceSize, fileSize uint64) *file {
	return &file{
		name:        name,
		size:        fileSize,
		contracts:   make(map[types.FileContractID]fileContract),
		masterKey:   crypto.GenerateTwofishKey(),
		erasureCode: code,
		pieceSize:   pieceSize,

		staticUID: persist.RandomSuffix(),
	}
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

	err := persist.RemoveFile(filepath.Join(r.persistDir, f.name+ShareExtension))
	if err != nil {
		r.log.Println("WARN: couldn't remove file :", err)
	}

	r.saveSync()
	r.mu.Unlock(lockID)

	// delete the file's associated contract data.
	f.mu.Lock()
	defer f.mu.Unlock()

	// mark the file as deleted
	f.deleted = true

	// TODO: delete the sectors of the file as well.

	return nil
}

// FileList returns all of the files that the renter has.
func (r *Renter) FileList() []modules.FileInfo {
	// Get all the files and their contracts
	var files []*file
	contractIDs := make(map[types.FileContractID]struct{})
	lockID := r.mu.RLock()
	for _, f := range r.files {
		files = append(files, f)
		f.mu.RLock()
		for cid := range f.contracts {
			contractIDs[cid] = struct{}{}
		}
		f.mu.RUnlock()
	}
	r.mu.RUnlock(lockID)

	// Build 2 maps that map every contract id to its offline and goodForRenew
	// status.
	goodForRenew := make(map[types.FileContractID]bool)
	offline := make(map[types.FileContractID]bool)
	for cid := range contractIDs {
		resolvedKey := r.hostContractor.ResolveIDToPubKey(cid)
		cu, ok := r.hostContractor.ContractUtility(resolvedKey)
		goodForRenew[cid] = ok && cu.GoodForRenew
		offline[cid] = r.hostContractor.IsOffline(resolvedKey)
	}

	// Build the list of FileInfos.
	var fileList []modules.FileInfo
	for _, f := range files {
		lockID := r.mu.RLock()
		f.mu.RLock()
		renewing := true
		var localPath string
		tf, exists := r.persist.Tracking[f.name]
		if exists {
			localPath = tf.RepairPath
		}
		fileList = append(fileList, modules.FileInfo{
			SiaPath:        f.name,
			LocalPath:      localPath,
			Filesize:       f.size,
			Renewing:       renewing,
			Available:      f.available(offline),
			Redundancy:     f.redundancy(offline, goodForRenew),
			UploadedBytes:  f.uploadedBytes(),
			UploadProgress: f.uploadProgress(),
			Expiration:     f.expiration(),
		})
		f.mu.RUnlock()
		r.mu.RUnlock(lockID)
	}
	return fileList
}

// File returns file from siaPath queried by user.
// Update based on FileList
func (r *Renter) File(siaPath string) (modules.FileInfo, error) {
	var fileInfo modules.FileInfo

	// Get the file and its contracs
	contractIDs := make(map[types.FileContractID]struct{})
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)
	file, exists := r.files[siaPath]
	if !exists {
		return fileInfo, ErrUnknownPath
	}
	file.mu.RLock()
	defer file.mu.RUnlock()
	for cid := range file.contracts {
		contractIDs[cid] = struct{}{}
	}

	// Build 2 maps that map every contract id to its offline and goodForRenew
	// status.
	goodForRenew := make(map[types.FileContractID]bool)
	offline := make(map[types.FileContractID]bool)
	for cid := range contractIDs {
		resolvedKey := r.hostContractor.ResolveIDToPubKey(cid)
		cu, ok := r.hostContractor.ContractUtility(resolvedKey)
		goodForRenew[cid] = ok && cu.GoodForRenew
		offline[cid] = r.hostContractor.IsOffline(resolvedKey)
	}

	// Build the FileInfo
	renewing := true
	var localPath string
	tf, exists := r.persist.Tracking[file.name]
	if exists {
		localPath = tf.RepairPath
	}
	fileInfo = modules.FileInfo{
		SiaPath:        file.name,
		LocalPath:      localPath,
		Filesize:       file.size,
		Renewing:       renewing,
		Available:      file.available(offline),
		Redundancy:     file.redundancy(offline, goodForRenew),
		UploadedBytes:  file.uploadedBytes(),
		UploadProgress: file.uploadProgress(),
		Expiration:     file.expiration(),
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
	file.mu.Lock()
	file.name = newName
	err = r.saveFile(file)
	file.mu.Unlock()
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
