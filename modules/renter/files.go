package renter

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrUnknownNickname  = errors.New("no file known by that nickname")
	ErrNicknameOverload = errors.New("a file with the proposed nickname already exists")
)

// A file is a single file that has been uploaded to the network. Files are
// split into equal-length chunks, which are then erasure-coded into pieces.
// Each piece is separately encrypted, using a key derived from the file's
// master key. The pieces are uploaded to hosts in groups, such that one file
// contract covers many pieces.
type file struct {
	name        string
	size        uint64
	contracts   map[types.FileContractID]fileContract
	masterKey   crypto.TwofishKey
	erasureCode modules.ErasureCoder
	pieceSize   uint64
	mode        uint32 // actually an os.FileMode
	mu          sync.RWMutex
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
type pieceData struct {
	Chunk  uint64 // which chunk the piece belongs to
	Piece  uint64 // the index of the piece in the chunk
	Offset uint64 // the offset of the piece in the file contract
}

// deriveKey derives the key used to encrypt and decrypt a specific file piece.
func deriveKey(masterKey crypto.TwofishKey, chunkIndex, pieceIndex uint64) crypto.TwofishKey {
	return crypto.TwofishKey(crypto.HashAll(masterKey, chunkIndex, pieceIndex))
}

// chunkSize returns the size of one chunk.
func (f *file) chunkSize() uint64 {
	return f.pieceSize * uint64(f.erasureCode.MinPieces())
}

// numChunks returns the number of chunks that f was split into.
func (f *file) numChunks() uint64 {
	n := f.size / f.chunkSize()
	if f.size%f.chunkSize() != 0 {
		n++
	}
	return n
}

// available indicates whether the file is ready to be downloaded.
func (f *file) available() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	chunkPieces := make([]int, f.numChunks())
	for _, fc := range f.contracts {
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

// uploadProgress indicates what percentage of the file (plus redundancy) has
// been uploaded. Note that a file may be Available long before UploadProgress
// reaches 100%, and UploadProgress may report a value greater than 100%.
func (f *file) uploadProgress() float32 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var uploaded uint64
	for _, fc := range f.contracts {
		uploaded += uint64(len(fc.Pieces)) * f.pieceSize
	}
	desired := f.pieceSize * uint64(f.erasureCode.NumPieces()) * f.numChunks()

	return 100 * (float32(uploaded) / float32(desired))
}

// expiration returns the lowest height at which any of the file's contracts
// will expire.
func (f *file) expiration() types.BlockHeight {
	f.mu.RLock()
	defer f.mu.RUnlock()
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
	key, _ := crypto.GenerateTwofishKey()
	return &file{
		name:        name,
		size:        fileSize,
		contracts:   make(map[types.FileContractID]fileContract),
		masterKey:   key,
		erasureCode: code,
		pieceSize:   pieceSize,
	}
}

// DeleteFile removes a file entry from the renter.
func (r *Renter) DeleteFile(nickname string) error {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	f, exists := r.files[nickname]
	if !exists {
		return ErrUnknownNickname
	}
	delete(r.files, nickname)

	os.Remove(filepath.Join(r.persistDir, f.name+ShareExtension))

	r.save()
	return nil
}

// FileList returns all of the files that the renter has.
func (r *Renter) FileList() []modules.FileInfo {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	files := make([]modules.FileInfo, 0, len(r.files))
	for _, f := range r.files {
		files = append(files, modules.FileInfo{
			Nickname:       f.name,
			Filesize:       f.size,
			Available:      f.available(),
			UploadProgress: f.uploadProgress(),
			Expiration:     f.expiration(),
		})
	}
	return files
}

// RenameFile takes an existing file and changes the nickname. The original
// file must exist, and there must not be any file that already has the
// replacement nickname.
func (r *Renter) RenameFile(currentName, newName string) error {
	return errors.New("renaming is disabled")

	/*
		lockID := r.mu.Lock()
		defer r.mu.Unlock(lockID)

		// Check that the currentName exists and the newName doesn't.
		file, exists := r.files[currentName]
		if !exists {
			return ErrUnknownNickname
		}
		_, exists = r.files[newName]
		if exists {
			return ErrNicknameOverload
		}

		// Do the renaming.
		file.name = newName
		r.saveFile(file)
		delete(r.files, currentName)
		r.files[newName] = file

		r.save()
		return nil

	*/
}
