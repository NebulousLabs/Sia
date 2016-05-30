package contractor

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errNilCS     = errors.New("cannot create contractor with nil consensus set")
	errNilWallet = errors.New("cannot create contractor with nil wallet")
	errNilTpool  = errors.New("cannot create contractor with nil transaction pool")
)

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

	allowance   modules.Allowance
	blockHeight types.BlockHeight
	contracts   map[types.FileContractID]modules.RenterContract
	lastChange  modules.ConsensusChangeID
	renewHeight types.BlockHeight // height at which to renew contracts

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

// SetAllowance sets the amount of money the Contractor is allowed to spend on
// contracts over a given time period, divided among the number of hosts
// specified. Note that Contractor can start forming contracts as soon as
// SetAllowance is called; that is, it may block.
//
// NOTE: At this time, transaction fees are not counted towards the allowance.
// This means the contractor may spend more than allowance.Funds.
func (c *Contractor) SetAllowance(a modules.Allowance) error {
	// sanity checks
	if a.Hosts == 0 {
		return errors.New("hosts must be non-zero")
	} else if a.Period == 0 {
		return errors.New("period must be non-zero")
	} else if a.RenewWindow == 0 {
		return errors.New("renew window must be non-zero")
	} else if a.RenewWindow >= a.Period {
		return errors.New("renew window must be less than period")
	}

	c.mu.RLock()
	// calculate number of new contracts to form
	var nContracts int
	if a.Hosts > uint64(len(c.contracts)) {
		nContracts = int(a.Hosts) - len(c.contracts)
	}
	c.mu.RUnlock()

	// only form contracts if we need to and have enough funds to do so
	if nContracts > 0 {
		err := c.managedFormContracts(nContracts, a)
		if err != nil {
			return err
		}
	}

	// Set the allowance.
	c.mu.Lock()
	c.allowance = a
	err := c.saveSync()
	c.mu.Unlock()

	return err
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

		contracts: make(map[types.FileContractID]modules.RenterContract),
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
