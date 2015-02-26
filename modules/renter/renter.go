package renter

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
)

// A FilePiece contains information about an individual file piece that has
// been uploaded to a host, including information about the host and the health
// of the file piece.
type FilePiece struct {
	Active     bool                     // Set to true if the host is online and has the file, false otherwise.
	Contract   consensus.FileContract   // The contract being enforced.
	ContractID consensus.FileContractID // The ID of the contract.
	HostIP     network.Address          // Where to find the file.
}

// A Renter is responsible for tracking all of the files that a user has
// uploaded to Sia, as well as the locations and health of these files.
type Renter struct {
	state  *consensus.State
	files  map[string][]FilePiece
	hostDB modules.HostDB
	wallet modules.Wallet

	mu sync.RWMutex
}

// New returns an empty renter.
func New(state *consensus.State, hdb modules.HostDB, wallet modules.Wallet) (r *Renter, err error) {
	if state == nil {
		err = errors.New("renter.New: cannot have nil state")
		return
	}
	if hdb == nil {
		err = errors.New("renter.New: cannot have nil hostDB")
		return
	}
	if wallet == nil {
		err = errors.New("renter.New: cannot have nil wallet")
		return
	}

	r = &Renter{
		state:  state,
		hostDB: hdb,
		wallet: wallet,
		files:  make(map[string][]FilePiece),
	}
	return
}

// Rename takes an existing file and changes the nickname. The original file
// must exist, and there must not be any file that already has the replacement
// nickname.
func (r *Renter) Rename(currentName, newName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check that the currentName exists and the newName doesn't.
	entry, exists := r.files[currentName]
	if !exists {
		return errors.New("no file found by that name")
	}
	_, exists = r.files[newName]
	if exists {
		return errors.New("file of new name already exists")
	}

	// Do the renaming.
	delete(r.files, currentName)
	r.files[newName] = entry
	return nil
}

// Info returns generic information about the renter and the files that are
// being rented.
func (r *Renter) Info() (ri modules.RentInfo) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for filename := range r.files {
		ri.Files = append(ri.Files, filename)
	}
	return
}
