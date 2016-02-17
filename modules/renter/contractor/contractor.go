package contractor

import (
	"errors"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errNilCS           = errors.New("cannot create contractor with nil consensus set")
	errNilWallet       = errors.New("cannot create contractor with nil wallet")
	errNilTpool        = errors.New("cannot create contractor with nil transaction pool")
	errUnknownContract = errors.New("no record of that contract")
)

// a hostContract includes the original contract made with a host, along with
// the most recent revision.
type hostContract struct {
	IP              modules.NetAddress
	ID              types.FileContractID
	FileContract    types.FileContract
	MerkleRoots     []crypto.Hash
	LastRevision    types.FileContractRevision
	LastRevisionTxn types.Transaction
	SecretKey       crypto.SecretKey
}

// A Contractor negotiates, revises, renews, and provides access to file
// contracts.
type Contractor struct {
	// dependencies
	dialer  dialer
	hdb     hostDB
	log     logger
	persist persister
	tpool   transactionPool
	wallet  wallet

	blockHeight   types.BlockHeight
	contracts     map[types.FileContractID]hostContract
	cachedAddress types.UnlockHash // to prevent excessive address creation

	mu sync.RWMutex
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
	logger, err := newLogger(persistDir)
	if err != nil {
		return nil, err
	}

	// Create Contractor using production dependencies.
	return newContractor(cs, &walletBridge{w: wallet}, tpool, hdb, stdDialer{}, newPersist(persistDir), logger)
}

// newContractor creates a Contractor using the provided dependencies.
func newContractor(cs consensusSet, w wallet, tp transactionPool, hdb hostDB, d dialer, p persister, l logger) (*Contractor, error) {
	// Create the Contractor object.
	c := &Contractor{
		dialer:  d,
		hdb:     hdb,
		log:     l,
		persist: p,
		tpool:   tp,
		wallet:  w,

		contracts: make(map[types.FileContractID]hostContract),
	}

	// Load the prior persistance structures.
	err := c.load()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	cs.ConsensusSetPersistentSubscribe(c, modules.ConsensusChangeID{})

	return c, nil
}
