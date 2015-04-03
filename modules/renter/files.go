package renter

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// A file is a single file that has been uploaded to the network.
type File struct {
	nickname    string
	pieces      []FilePiece
	startHeight types.BlockHeight

	renter *Renter
}

// A FilePiece contains information about an individual file piece that has
// been uploaded to a host, including information about the host and the health
// of the file piece.
type FilePiece struct {
	Active     bool                 // Set to true if the host is online and has the file, false otherwise.
	Repairing  bool                 // Set to true if there's an upload happening for the piece at the moment.
	Contract   types.FileContract   // The contract being enforced.
	ContractID types.FileContractID // The ID of the contract.
	HostIP     modules.NetAddress   // Where to find the file.
}

// Available indicates whether the file is ready to be downloaded.
func (f *File) Available() bool {
	f.renter.mu.RLock()
	defer f.renter.mu.RUnlock()

	for _, piece := range f.pieces {
		if piece.Active {
			return true
		}
	}
	return false
}

// Nickname returns the nickname of the file.
func (f *File) Nickname() string {
	f.renter.mu.RLock()
	defer f.renter.mu.RUnlock()
	return f.nickname
}

// Repairing returns whether or not the file is actively being repaired.
func (f *File) Repairing() bool {
	f.renter.mu.RLock()
	defer f.renter.mu.RUnlock()

	for _, piece := range f.pieces {
		if piece.Repairing {
			return true
		}
	}
	return false
}

// TimeRemaining returns the amount of time until the file's contracts expire.
func (f *File) TimeRemaining() types.BlockHeight {
	f.renter.mu.RLock()
	defer f.renter.mu.RUnlock()
	return f.startHeight - f.renter.state.Height()
}

// FileList returns all of the files that the renter has.
func (r *Renter) FileList() (files []modules.FileInfo) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, file := range r.files {
		// Because 'file' is the same memory for all iterations, we need to
		// make a copy.
		f := &File{
			nickname:    file.nickname,
			pieces:      file.pieces,
			startHeight: file.startHeight,
			renter:      file.renter,
		}
		files = append(files, f)
	}
	return
}
