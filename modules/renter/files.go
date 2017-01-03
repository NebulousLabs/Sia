package renter

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrEmptyFilename = errors.New("filename must be a nonempty string")
	ErrUnknownPath   = errors.New("no file known with that path")
	ErrPathOverload  = errors.New("a file already exists at that location")
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
	Chunk      uint64      // which chunk the piece belongs to
	Piece      uint64      // the index of the piece in the chunk
	MerkleRoot crypto.Hash // the Merkle root of the piece
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
	// empty files still need at least one chunk
	if f.size == 0 {
		return 1
	}
	n := f.size / f.chunkSize()
	// last chunk will be padded, unless chunkSize divides file evenly.
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
func (f *file) uploadProgress() float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var uploaded uint64
	for _, fc := range f.contracts {
		uploaded += uint64(len(fc.Pieces)) * f.pieceSize
	}
	desired := f.pieceSize * uint64(f.erasureCode.NumPieces()) * f.numChunks()

	return 100 * (float64(uploaded) / float64(desired))
}

// redundancy returns the redundancy of the least redundant chunk. A file
// becomes available when this redundancy is >= 1. Assumes that every piece is
// unique within a file contract. -1 is returned if the file has size 0.
func (f *file) redundancy() float64 {
	if f.size == 0 {
		return -1
	}
	piecesPerChunk := make([]int, f.numChunks())
	// If the file has non-0 size then the number of chunks should also be
	// non-0. Therefore the f.size == 0 conditional block above must appear
	// before this check.
	if len(piecesPerChunk) == 0 {
		build.Critical("cannot get redundancy of a file with 0 chunks")
		return -1
	}
	for _, fc := range f.contracts {
		for _, p := range fc.Pieces {
			piecesPerChunk[p.Chunk]++
		}
	}
	minPieces := piecesPerChunk[0]
	for _, numPieces := range piecesPerChunk {
		if numPieces < minPieces {
			minPieces = numPieces
		}
	}
	return float64(minPieces) / float64(f.erasureCode.MinPieces())
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

// DeleteFile removes a file entry from the renter and deletes its data from
// the hosts it is stored on.
func (r *Renter) DeleteFile(nickname string) error {
	lockID := r.mu.Lock()
	f, exists := r.files[nickname]
	if !exists {
		r.mu.Unlock(lockID)
		return ErrUnknownPath
	}
	delete(r.files, nickname)
	os.RemoveAll(filepath.Join(r.persistDir, f.name+ShareExtension))
	r.saveSync()
	r.mu.Unlock(lockID)

	// delete the file's associated contract data.
	f.mu.Lock()
	defer f.mu.Unlock()

	// TODO: this is ugly because we only have the Contracts method for
	// looking up contracts.
	var contracts []modules.RenterContract
	for _, c := range r.hostContractor.Contracts() {
		if _, ok := f.contracts[c.ID]; ok {
			contracts = append(contracts, c)
		}
	}
	for _, c := range contracts {
		editor, err := r.hostContractor.Editor(c.ID)
		if err != nil {
			// TODO: what if the host isn't online?
			continue
		}
		for _, root := range c.MerkleRoots {
			editor.Delete(root)
		}
		delete(f.contracts, c.ID)
	}

	return nil
}

// FileList returns all of the files that the renter has.
func (r *Renter) FileList() []modules.FileInfo {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	files := make([]modules.FileInfo, 0, len(r.files))
	for _, f := range r.files {
		// _, renewing := r.tracking[f.name]
		// TODO: bring back per-file renewing
		renewing := true
		files = append(files, modules.FileInfo{
			SiaPath:        f.name,
			Filesize:       f.size,
			Available:      f.available(),
			Redundancy:     f.redundancy(),
			Renewing:       renewing,
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
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	// Check that newName is nonempty.
	if newName == "" {
		return ErrEmptyFilename
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
	err := r.saveFile(file)
	file.mu.Unlock()
	if err != nil {
		return err
	}

	// Update the entries in the renter.
	delete(r.files, currentName)
	r.files[newName] = file
	if t, ok := r.tracking[currentName]; ok {
		delete(r.tracking, currentName)
		r.tracking[newName] = t
	}
	err = r.saveSync()
	if err != nil {
		return err
	}

	// Delete the old .sia file.
	// NOTE: proper error handling is difficult here. For example, if the
	// removal fails, should the entry in r.files be preserved? For now we will
	// keep things simple, but it is important that our approach feels
	// intuitive/unsurprising and doesn't put the user's data at risk.
	oldPath := filepath.Join(r.persistDir, currentName+ShareExtension)
	return os.RemoveAll(oldPath)
}
