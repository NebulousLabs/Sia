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
	Active     bool                 // True if the host has the file and has been online somewhat recently.
	Repairing  bool                 // True if the piece is currently being uploaded.
	Contract   types.FileContract   // The contract being enforced.
	ContractID types.FileContractID // The ID of the contract.

	HostIP     modules.NetAddress // Where to find the file piece.
	StartIndex uint64
	EndIndex   uint64

	Transferred uint64
	PieceSize   uint64

	PieceIndex    int // Indicates the erasure coding index of this piece.
	EncryptionKey crypto.TwofishKey
	Checksum      crypto.Hash
}

// Available indicates whether the file is ready to be downloaded.
func (f *file) Available() bool {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)

	var active int
	for _, piece := range f.Pieces {
		if piece.Active {
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
	var max float32
	for _, piece := range f.Pieces {
		progress := float32(piece.Transferred) / float32(piece.PieceSize)
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

	for _, piece := range f.Pieces {
		if piece.Repairing {
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
	for _, piece := range f.Pieces {
		if piece.Contract.WindowStart < f.renter.blockHeight {
			continue
		}
		current := piece.Contract.WindowStart - f.renter.blockHeight
		if current > largest {
			largest = current
		}
	}
	return largest
}

// DeleteFile removes a file entry from the renter.
func (r *Renter) DeleteFile(nickname string) error {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

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
