package contractor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	siasync "github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errNilCS     = errors.New("cannot create contractor with nil consensus set")
	errNilWallet = errors.New("cannot create contractor with nil wallet")
	errNilTpool  = errors.New("cannot create contractor with nil transaction pool")

	// COMPATv1.0.4-lts
	// metricsContractID identifies a special contract that contains aggregate
	// financial metrics from older contractors
	metricsContractID = types.FileContractID{'m', 'e', 't', 'r', 'i', 'c', 's'}
)

// A cachedRevision contains changes that would be applied to a RenterContract
// if a contract revision succeeded. The contractor must cache these changes
// as a safeguard against desynchronizing with the host.
// TODO: save a diff of the Merkle roots instead of all of them.
type cachedRevision struct {
	Revision    types.FileContractRevision `json:"revision"`
	MerkleRoots modules.MerkleRootSet      `json:"merkleroots"`
}

// A Contractor negotiates, revises, renews, and provides access to file
// contracts.
type Contractor struct {
	// dependencies
	cs      consensusSet
	hdb     hostDB
	log     *persist.Logger
	persist persister
	mu      sync.RWMutex
	tg      siasync.ThreadGroup
	tpool   transactionPool
	wallet  wallet

	// Only one thread should be running contract repair at a time.
	editLock           siasync.TryMutex
	contractRepairLock siasync.TryMutex

	allowance     modules.Allowance
	blockHeight   types.BlockHeight
	currentPeriod types.BlockHeight
	lastChange    modules.ConsensusChangeID

	downloaders map[types.FileContractID]*hostDownloader
	editors     map[types.FileContractID]*hostEditor

	cachedRevisions map[types.FileContractID]cachedRevision
	contractLocks   map[types.FileContractID]*siasync.TryMutex
	contracts       map[types.FileContractID]modules.RenterContract
	oldContracts    map[types.FileContractID]modules.RenterContract
	renewedIDs      map[types.FileContractID]types.FileContractID
}

// Allowance returns the current allowance.
func (c *Contractor) Allowance() modules.Allowance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.allowance
}

// AllContracts returns the contracts formed by the contractor in the current
// allowance period.
func (c *Contractor) AllContracts() (cs []modules.RenterContract) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, contract := range c.contracts {
		cs = append(cs, contract)
	}
	// COMPATv1.0.4-lts
	// also return the special metrics contract (see persist.go)
	if contract, ok := c.oldContracts[metricsContractID]; ok {
		cs = append(cs, contract)
	}
	return
}

// Close closes the Contractor.
func (c *Contractor) Close() error {
	return c.tg.Stop()
}

// Contract returns the latest contract formed with the specified host.
func (c *Contractor) Contract(id types.FileContractID) (modules.RenterContract, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	contract, exists := c.contracts[id]
	return contract, exists
}

// Contracts returns the contracts formed by the contractor in the current
// allowance period. Only contracts formed with currently online hosts are
// returned.
func (c *Contractor) Contracts() []modules.RenterContract {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cs := make([]modules.RenterContract, 0, len(c.contracts))
	for _, contract := range c.contracts {
		cs = append(cs, contract)
	}
	return cs
}

// CurrentPeriod returns the height at which the current allowance period
// began.
func (c *Contractor) CurrentPeriod() types.BlockHeight {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentPeriod
}

// ResolveID returns the ID of the most recent renewal of id.
func (c *Contractor) ResolveID(id types.FileContractID) types.FileContractID {
	if newID, ok := c.renewedIDs[id]; ok && newID != id {
		return c.ResolveID(newID)
	}
	return id
}

// New returns a new Contractor.
func New(cs consensusSet, wallet walletShim, tpool transactionPool, hdb hostDB, persistDir string) (*Contractor, error) {
	// Check for nil inputs.
	if cs == nil {
		return nil, errNilCS
	}
	if wallet == nil {
		return nil, errNilWallet
	}
	if tpool == nil {
		return nil, errNilTpool
	}

	// Create the persist directory if it does not yet exist.
	err := os.MkdirAll(persistDir, 0700)
	if err != nil {
		return nil, err
	}
	// Create the logger.
	logger, err := persist.NewFileLogger(filepath.Join(persistDir, "contractor.log"))
	if err != nil {
		return nil, err
	}

	// Create Contractor using production dependencies.
	return newContractor(cs, &walletBridge{w: wallet}, tpool, hdb, newPersist(persistDir), logger)
}

// newContractor creates a Contractor using the provided dependencies.
func newContractor(cs consensusSet, w wallet, tp transactionPool, hdb hostDB, p persister, l *persist.Logger) (*Contractor, error) {
	// Create the Contractor object.
	c := &Contractor{
		cs:      cs,
		hdb:     hdb,
		log:     l,
		persist: p,
		tpool:   tp,
		wallet:  w,

		cachedRevisions: make(map[types.FileContractID]cachedRevision),
		contracts:       make(map[types.FileContractID]modules.RenterContract),
		downloaders:     make(map[types.FileContractID]*hostDownloader),
		editors:         make(map[types.FileContractID]*hostEditor),
		oldContracts:    make(map[types.FileContractID]modules.RenterContract),
		renewedIDs:      make(map[types.FileContractID]types.FileContractID),
	}

	// Close the logger (provided as a dependency) upon shutdown.
	c.tg.AfterStop(func() {
		if err := c.log.Close(); err != nil {
			fmt.Println("Failed to close the contractor logger:", err)
		}
	})

	// Load the prior persistence structures.
	err := c.load()
	if err != nil && !os.IsNotExist(err) {
		closeErr := c.Close()
		if closeErr != nil {
			fmt.Println("Unable to close contractor safely:", err)
		}
		return nil, err
	}
	// Close the persist (provided as a dependency) upon shutdown.
	c.tg.AfterStop(func() {
		if err := c.persist.Close(); err != nil {
			c.log.Println("Failed to close contractor persist:", err)
		}
	})

	// Subscribe to the consensus set.
	err = cs.ConsensusSetSubscribe(c, c.lastChange)
	if err == modules.ErrInvalidConsensusChangeID {
		// Reset the contractor consensus variables and try rescanning.
		c.blockHeight = 0
		c.lastChange = modules.ConsensusChangeBeginning
		err = cs.ConsensusSetSubscribe(c, c.lastChange)
	}
	if err != nil {
		closeErr := c.Close()
		if closeErr != nil {
			fmt.Println("Unable to close contractor safely:", err)
		}
		return nil, errors.New("contractor subscription failed: " + err.Error())
	}
	// Unsubscribe from the consensus set upon shutdown.
	c.tg.OnStop(func() {
		cs.Unsubscribe(c)
	})

	// We may have upgraded persist or resubscribed. Save now so that we don't
	// lose our work.
	c.mu.Lock()
	err = c.save()
	c.mu.Unlock()
	if err != nil {
		closeErr := c.Close()
		if closeErr != nil {
			fmt.Println("Unable to close contractor safely:", err)
		}
		return nil, err
	}
	return c, nil
}
