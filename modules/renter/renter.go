package renter

import (
	"errors"
	"os"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

// A Renter is responsible for tracking all of the files that a user has
// uploaded to Sia, as well as the locations and health of these files.
type Renter struct {
	cs          *consensus.State
	gateway     modules.Gateway
	hostDB      modules.HostDB
	wallet      modules.Wallet
	blockHeight types.BlockHeight

	files         map[string]File
	downloadQueue []*Download
	saveDir       string

	subscriptions []chan struct{}

	mu *sync.RWMutex
}

// New returns an empty renter.
func New(cs *consensus.State, gateway modules.Gateway, hdb modules.HostDB, wallet modules.Wallet, saveDir string) (r *Renter, err error) {
	if cs == nil {
		err = errors.New("renter.New: cannot have nil consensus set")
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
		cs:      cs,
		gateway: gateway,
		hostDB:  hdb,
		wallet:  wallet,

		files:   make(map[string]File),
		saveDir: saveDir,

		mu: sync.New(1*time.Second, 0),
	}

	err = os.MkdirAll(saveDir, 0700)
	if err != nil {
		return
	}

	r.load()

	// TODO: I'm worried about balances here. Because of the way that the
	// re-try algorithm works, it won't be a problem, but without that we would
	// need to make sure that scanAllFiles() didn't get called until the entire
	// balance had loaded, which would require loading the entire blockchain.
	// This also won't be a problem once we're also saving the addresses.
	go r.scanAllFiles()

	r.cs.ConsensusSetSubscribe(r)

	return
}

// Rename takes an existing file and changes the nickname. The original file
// must exist, and there must not be any file that already has the replacement
// nickname.
func (r *Renter) Rename(currentName, newName string) error {
	lockID := r.mu.Lock()
	defer r.mu.Unlock(lockID)

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
	lockID := r.mu.RLock()
	defer r.mu.RUnlock(lockID)

	for filename := range r.files {
		ri.Files = append(ri.Files, filename)
	}
	return
}
