package renter

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

type FilePiece struct {
	Host       modules.HostEntry      // Where to find the file.
	Contract   consensus.FileContract // The contract being enforced.
	ContractID consensus.ContractID   // The ID of the contract.
}

type Renter struct {
	state  *consensus.State
	files  map[string][]FilePiece
	hostDB modules.HostDB
	wallet modules.Wallet

	mu sync.RWMutex
}

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

func (r *Renter) Rename(currentName, newName string) error {
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

func (r *Renter) Info() (ri modules.RentInfo) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for filename := range r.files {
		ri.Files = append(ri.Files, filename)
	}
	return
}
