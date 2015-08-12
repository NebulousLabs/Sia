package renter

import (
	"errors"
	"sync/atomic"

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
type dfile struct {
	Name      string
	Size      uint64
	Contracts map[modules.NetAddress]fileContract
	MasterKey crypto.TwofishKey
	ecc       modules.ECC
	pieceSize uint64

	uploaded uint64
}

// chunkSize returns the size of one chunk.
func (f *dfile) chunkSize() uint64 {
	return f.pieceSize * uint64(f.ecc.NumPieces())
}

// numChunks returns the number of chunks that f was split into.
func (f *dfile) numChunks() uint64 {
	n := f.Size / f.chunkSize()
	if f.Size%f.chunkSize() != 0 {
		n++
	}
	return n
}

// A fileContract is a contract covering an arbitrary number of file pieces.
// Chunk/Piece metadata is used to split the raw contract data appropriately.
type fileContract struct {
	types.FileContract // TODO: store this internally in the renter

	ID     types.FileContractID
	IP     modules.NetAddress
	Pieces []pieceData
}

// pieceData contains the metadata necessary to request a piece from a
// fetcher.
type pieceData struct {
	Chunk  uint64 // which chunk the piece belongs to
	Piece  uint64 // the index of the piece in the chunk
	Offset uint64 // the offset of the piece in the file contract
	Length uint64 // the length of the piece
}

// Available indicates whether the file is ready to be downloaded.
func (f *dfile) Available() bool {
	// TODO: what's the best way to do this?
	return true
}

// UploadProgress indicates what percentage of the file (plus redundancy) has
// been uploaded. Note that a file may be Available long before UploadProgress
// reaches 100%.
func (f *dfile) UploadProgress() float32 {
	return 0
}

// Nickname returns the nickname of the file.
func (f *dfile) Nickname() string {
	return f.Name
}

// Filesize returns the size of the file.
func (f *dfile) Filesize() uint64 {
	return f.Size
}

// Repairing returns whether or not the file is actively being repaired.
func (f *dfile) Repairing() bool {
	return false
}

// Expiration returns the lowest height at which any of the file's contracts
// will expire.
func (f *dfile) Expiration() types.BlockHeight {
	if len(f.Contracts) == 0 {
		return 0
	}
	lowest := types.BlockHeight(0)
	for _, fc := range f.Contracts {
		if fc.WindowStart < lowest {
			lowest = fc.WindowStart
		}
	}
	return lowest
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

// A file is a single file that has been uploaded to the network.
type file struct {
	Name     string
	Checksum crypto.Hash // checksum of the decoded file.

	// Erasure coding variables:
	//		piecesRequired <= optimalRecoveryPieces <= totalPieces
	ErasureScheme         string
	PiecesRequired        int
	OptimalRecoveryPieces int
	TotalPieces           int
	Pieces                []filePiece

	// DEPRECATED - the new renter scheme has the renter pre-making contracts
	// with hosts uploading new contracts through diffs.
	UploadParams modules.FileUploadParams

	// The file needs to access the renter's lock. This variable is not
	// exported so that the persistence functions won't save the whole renter.
	renter *Renter
}

// A filePiece contains information about an individual file piece that has
// been uploaded to a host, including information about the host and the health
// of the file piece.
type filePiece struct {
	// Implementation node: 'Transferred' is declared first to ensure that it
	// is 64-byte aligned. This is necessary to ensure that atomic operations
	// work correctly on ARM and x86-32.
	Transferred uint64

	Active     bool                 // True if the host has the file and has been online somewhat recently.
	Repairing  bool                 // True if the piece is currently being uploaded.
	Contract   types.FileContract   // The contract being enforced.
	ContractID types.FileContractID // The ID of the contract.

	HostIP     modules.NetAddress // Where to find the file piece.
	StartIndex uint64
	EndIndex   uint64

	PieceSize uint64

	PieceIndex    int // Indicates the erasure coding index of this piece.
	EncryptionKey crypto.TwofishKey
	Checksum      crypto.Hash
}

// Available indicates whether the file is ready to be downloaded.
func (f *file) Available() bool {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)

	// The loop uses an index instead of a range because range copies the piece
	// to fresh data. Atomic operations are being concurrently performed on the
	// piece, and the copy results in a race condition against the atomic
	// operations. By removing the copying, the race condition is eliminated.
	var active int
	for i := range f.Pieces {
		if f.Pieces[i].Active {
			active++
		}
		if active >= f.PiecesRequired {
			return true
		}
	}
	return false
}

// UploadProgress indicates how close the file is to being available.
func (f *file) UploadProgress() float32 {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)

	// full replication means we just use the progress of most-uploaded piece.
	//
	// The loop uses an index instead of a range because range copies the piece
	// to fresh data. Atomic operations are being concurrently performed on the
	// piece, and the copy results in a race condition against the atomic
	// operations. By removing the copying, the race condition is eliminated.
	var max float32
	for i := range f.Pieces {
		progress := float32(atomic.LoadUint64(&f.Pieces[i].Transferred)) / float32(f.Pieces[i].PieceSize)
		if progress > max {
			max = progress
		}
	}
	return 100 * max
}

// Nickname returns the nickname of the file.
func (f *file) Nickname() string {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)
	return f.Name
}

// Filesize returns the size of the file.
func (f *file) Filesize() uint64 {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)
	// TODO: this will break when we switch to erasure coding.
	for i := range f.Pieces {
		if f.Pieces[i].Contract.FileSize != 0 {
			return f.Pieces[i].Contract.FileSize
		}
	}
	return 0
}

// Repairing returns whether or not the file is actively being repaired.
func (f *file) Repairing() bool {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)

	for i := range f.Pieces {
		if f.Pieces[i].Repairing {
			return true
		}
	}
	return false
}

// TimeRemaining returns the amount of time until the file's contracts expire.
func (f *file) TimeRemaining() types.BlockHeight {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)

	largest := types.BlockHeight(0)
	for i := range f.Pieces {
		if f.Pieces[i].Contract.WindowStart < f.renter.blockHeight {
			continue
		}
		current := f.Pieces[i].Contract.WindowStart - f.renter.blockHeight
		if current > largest {
			largest = current
		}
	}
	return largest
}

func (f *file) Expiration() types.BlockHeight {
	return 0
}
