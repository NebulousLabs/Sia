package renter

import (
	"errors"

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
	Name      string
	Size      uint64
	Contracts map[modules.NetAddress]fileContract
	MasterKey crypto.TwofishKey
	ecc       modules.ECC
	pieceSize uint64

	bytesUploaded  uint64
	chunksUploaded uint64
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
	Length uint64 // the length of the piece
}

// chunkSize returns the size of one chunk.
func (f *file) chunkSize() uint64 {
	return f.pieceSize * uint64(f.ecc.MinPieces())
}

// numChunks returns the number of chunks that f was split into.
func (f *file) numChunks() uint64 {
	n := f.Size / f.chunkSize()
	if f.Size%f.chunkSize() != 0 {
		n++
	}
	return n
}

// Available indicates whether the file is ready to be downloaded.
func (f *file) Available() bool {
	return f.chunksUploaded >= f.numChunks()
}

// UploadProgress indicates what percentage of the file (plus redundancy) has
// been uploaded. Note that a file may be Available long before UploadProgress
// reaches 100%.
func (f *file) UploadProgress() float32 {
	totalBytes := f.pieceSize * uint64(f.ecc.NumPieces()) * f.numChunks()
	return 100 * float32(f.bytesUploaded) / float32(totalBytes)
}

// Nickname returns the nickname of the file.
func (f *file) Nickname() string {
	return f.Name
}

// Filesize returns the size of the file.
func (f *file) Filesize() uint64 {
	return f.Size
}

// Expiration returns the lowest height at which any of the file's contracts
// will expire.
func (f *file) Expiration() types.BlockHeight {
	if len(f.Contracts) == 0 {
		return 0
	}
	lowest := ^types.BlockHeight(0)
	for _, fc := range f.Contracts {
		if fc.WindowStart < lowest {
			lowest = fc.WindowStart
		}
	}
	return lowest
}

// newFile creates a new file object.
func newFile(ecc modules.ECC, pieceSize, fileSize uint64) *file {
	key, _ := crypto.GenerateTwofishKey()
	return &file{
		Size:      fileSize,
		Contracts: make(map[modules.NetAddress]fileContract),
		MasterKey: key,
		ecc:       ecc,
		pieceSize: pieceSize,
	}
}

// DeleteFile removes a file entry from the renter.
func (r *Renter) DeleteFile(nickname string) error {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

	_, exists := r.files[nickname]
	if !exists {
		return ErrUnknownNickname
	}
	delete(r.files, nickname)

	r.save()
	return nil
}

// FileList returns all of the files that the renter has.
func (r *Renter) FileList() (files []modules.FileInfo) {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	for _, f := range r.files {
		files = append(files, f)
	}
	return
}

// RenameFile takes an existing file and changes the nickname. The original
// file must exist, and there must not be any file that already has the
// replacement nickname.
func (r *Renter) RenameFile(currentName, newName string) error {
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
	delete(r.files, currentName)
	file.Name = newName
	r.files[newName] = file

	r.save()
	return nil
}
