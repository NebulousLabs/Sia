package renter

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// A File is a single file that has been uploaded to the network.
type File struct {
	nickname     string
	pieces       []FilePiece
	uploadParams modules.UploadParams
	checksum     crypto.Hash

	// The File needs to access the Renter's lock.
	renter *Renter
}

// A FilePiece contains information about an individual file piece that has
// been uploaded to a host, including information about the host and the health
// of the file piece.
type FilePiece struct {
	Active     bool                 // True if the host has the file and has been online somewhat recently.
	Repairing  bool                 // True if the piece is currently being uploaded.
	Contract   types.FileContract   // The contract being enforced.
	ContractID types.FileContractID // The ID of the contract.

	HostIP     modules.NetAddress // Where to find the file.
	StartIndex uint64
	EndIndex   uint64

	PieceIndex int // Indicates the erasure coding index of this piece.
	Checksum   crypto.Hash
}

// Available indicates whether the file is ready to be downloaded.
func (f *File) Available() bool {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)

	for _, piece := range f.pieces {
		if piece.Active {
			return true
		}
	}
	return false
}

// Nickname returns the nickname of the file.
func (f *File) Nickname() string {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)
	return f.nickname
}

// Repairing returns whether or not the file is actively being repaired.
func (f *File) Repairing() bool {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)

	for _, piece := range f.pieces {
		if piece.Repairing {
			return true
		}
	}
	return false
}

// TimeRemaining returns the amount of time until the file's contracts expire.
func (f *File) TimeRemaining() types.BlockHeight {
	lockID := f.renter.mu.RLock()
	defer f.renter.mu.RUnlock(lockID)

	if len(f.pieces) == 0 {
		return 0
	}
	if f.pieces[0].Contract.WindowStart < f.renter.blockHeight {
		return 0
	}
	return f.pieces[0].Contract.WindowStart - f.renter.blockHeight
}

// FileList returns all of the files that the renter has.
func (r *Renter) FileList() (files []modules.FileInfo) {
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	for _, file := range r.files {
		// Because 'file' is the same memory for all iterations, we need to
		// make a copy.
		f := File{
			nickname:     file.nickname,
			pieces:       file.pieces,
			uploadParams: file.uploadParams,
			checksum:     file.checksum,

			renter: file.renter,
		}
		files = append(files, &f)
	}
	return
}
