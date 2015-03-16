package renter

import (
	"errors"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

// A Renter is responsible for tracking all of the files that a user has
// uploaded to Sia, as well as the locations and health of these files.
type Renter struct {
	state   *consensus.State
	gateway modules.Gateway
	hostDB  modules.HostDB
	wallet  modules.Wallet

	files         map[string]File
	downloadQueue []Download
	saveDir       string

	mu sync.RWMutex
}

// New returns an empty renter.
func New(state *consensus.State, gateway modules.Gateway, hdb modules.HostDB, wallet modules.Wallet, saveDir string) (r *Renter, err error) {
	if state == nil {
		err = errors.New("renter.New: cannot have nil state")
		return
	}
	if gateway == nil {
		err = errors.New("renter.New: cannot have nil gateway")
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
		state:   state,
		gateway: gateway,
		hostDB:  hdb,
		wallet:  wallet,
		files:   make(map[string]File),
		saveDir: saveDir,
	}

	err = os.MkdirAll(saveDir, 0700)
	if err != nil {
		return
	}

	r.load()

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
	entry.nickname = newName
	r.files[newName] = entry

	r.save()
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
