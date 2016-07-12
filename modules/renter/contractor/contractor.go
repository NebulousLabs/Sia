package contractor

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errNilCS     = errors.New("cannot create contractor with nil consensus set")
	errNilWallet = errors.New("cannot create contractor with nil wallet")
	errNilTpool  = errors.New("cannot create contractor with nil transaction pool")
)

// A cachedRevision contains changes that would be applied to a RenterContract
// if a contract revision succeeded. The contractor must cache these changes
// as a safeguard against desynchronizing with the host.
// TODO: save a diff of the Merkle roots instead of all of them.
type cachedRevision struct {
	revision    types.FileContractRevision
	merkleRoots []crypto.Hash
}

// A Contractor negotiates, revises, renews, and provides access to file
// contracts.
type Contractor struct {
	// dependencies
	cs      consensusSet
	hdb     hostDB
	log     *persist.Logger
	persist persister
	tpool   transactionPool
	wallet  wallet

	allowance       modules.Allowance
	blockHeight     types.BlockHeight
	cachedRevisions map[types.FileContractID]cachedRevision
	contracts       map[types.FileContractID]modules.RenterContract
	lastChange      modules.ConsensusChangeID
	renewHeight     types.BlockHeight // height at which to renew contracts

	financialMetrics modules.RenterFinancialMetrics

	mu sync.RWMutex
}

// Allowance returns the current allowance.
func (c *Contractor) Allowance() modules.Allowance {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.allowance
}

// FinancialMetrics returns the financial metrics of the Contractor.
func (c *Contractor) FinancialMetrics() modules.RenterFinancialMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.financialMetrics
}

// Contracts returns the contracts formed by the contractor.
func (c *Contractor) Contracts() (cs []modules.RenterContract) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, c := range c.contracts {
		cs = append(cs, c)
	}
	return
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
	}

	// Load the prior persistence structures.
	err := c.load()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	err = cs.ConsensusSetSubscribe(c, c.lastChange)
	if err == modules.ErrInvalidConsensusChangeID {
		c.lastChange = modules.ConsensusChangeBeginning
		// ??? fix things ???
		// subscribe again using the new ID
		err = cs.ConsensusSetSubscribe(c, c.lastChange)
	}
	if err != nil {
		return nil, errors.New("contractor subscription failed: " + err.Error())
	}

	return c, nil
}
