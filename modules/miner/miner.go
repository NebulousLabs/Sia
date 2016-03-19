package miner

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errNilCS     = errors.New("miner cannot use a nil consensus set")
	errNilTpool  = errors.New("miner cannot use a nil transaction pool")
	errNilWallet = errors.New("miner cannot use a nil wallet")

	// HeaderMemory is the number of previous calls to 'header'
	// that are remembered. Additionally, 'header' will only poll for a
	// new block every 'headerMemory / blockMemory' times it is
	// called. This reduces the amount of memory used, but comes at the cost of
	// not always having the most recent transactions.
	HeaderMemory = func() int {
		if build.Release == "dev" {
			return 500
		}
		if build.Release == "standard" {
			return 10000
		}
		if build.Release == "testing" {
			return 50
		}
		panic("unrecognized build.Release")
	}()

	// BlockMemory is the maximum number of blocks the miner will store
	// Blocks take up to 2 megabytes of memory, which is why this number is
	// limited.
	BlockMemory = func() int {
		if build.Release == "dev" {
			return 10
		}
		if build.Release == "standard" {
			return 50
		}
		if build.Release == "testing" {
			return 5
		}
		panic("unrecognized build.Release")
	}()

	// MaxSourceBlockAge is the maximum amount of time that is allowed to
	// elapse between generating source blocks.
	MaxSourceBlockAge = func() time.Duration {
		if build.Release == "dev" {
			return 5 * time.Second
		}
		if build.Release == "standard" {
			return 30 * time.Second
		}
		if build.Release == "testing" {
			return 1 * time.Second
		}
		panic("unrecognized build.Release")
	}()
)

// Miner struct contains all variables the miner needs
// in order to create and submit blocks.
type Miner struct {
	// Module dependencies.
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	wallet modules.Wallet

	// BlockManager variables. Becaues blocks are large, one block is used to
	// make many headers which can be used by miners. Headers include an
	// arbitrary data transaction (appended to the block) to make the merkle
	// roots unique (preventing miners from doing redundant work). Every N
	// requests or M seconds, a new block is used to create headers.
	//
	// Only 'blocksMemory' blocks are kept in memory at a time, which
	// keeps ram usage reasonable. Miners may request many headers in parallel,
	// and thus may be working on different blocks. When they submit the solved
	// header to the block manager, the rest of the block needs to be found in
	// a lookup.
	blockMem        map[types.BlockHeader]*types.Block             // Mappings from headers to the blocks they are derived from.
	arbDataMem      map[types.BlockHeader][crypto.EntropySize]byte // Mappings from the headers to their unique arb data.
	headerMem       []types.BlockHeader                            // A circular list of headers that have been given out from the api recently.
	sourceBlock     *types.Block                                   // The block from which new headers for mining are created.
	sourceBlockTime time.Time                                      // How long headers have been using the same block (different from 'recent block').
	memProgress     int                                            // The index of the most recent header used in headerMem.

	// CPUMiner variables.
	miningOn bool  // indicates if the miner is supposed to be running
	mining   bool  // indicates if the miner is actually running
	hashRate int64 // indicates hashes per second

	// Utils
	log        *persist.Logger
	mu         sync.RWMutex
	persist    persistence
	persistDir string
}

// startupRescan will rescan the blockchain in the event that the miner
// persistance layer has become desynchronized from the consensus persistance
// layer. This might happen if a user replaces any of the folders with backups
// or deletes any of the folders.
func (m *Miner) startupRescan() error {
	// Reset all of the variables that have relevance to the consensus set. The
	// operations are wrapped by an anonymous function so that the locking can
	// be handled using a defer statement.
	err := func() error {
		m.mu.Lock()
		defer m.mu.Unlock()

		m.persist.RecentChange = modules.ConsensusChangeID{}
		m.persist.Height = 0
		m.persist.Target = types.Target{}
		return m.save()
	}()
	if err != nil {
		return err
	}

	// Subscribe to the consensus set. This is a blocking call that will not
	// return until the miner has fully caught up to the current block.
	return m.cs.ConsensusSetPersistentSubscribe(m, modules.ConsensusChangeID{})
}

// New returns a ready-to-go miner that is not mining.
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, w modules.Wallet, persistDir string) (*Miner, error) {
	// Create the miner and its dependencies.
	if cs == nil {
		return nil, errNilCS
	}
	if tpool == nil {
		return nil, errNilTpool
	}
	if w == nil {
		return nil, errNilWallet
	}

	// Assemble the miner. The miner is assembled without an address because
	// the wallet is likely not unlocked yet. The miner will grab an address
	// after the miner is unlocked (this must be coded manually for each
	// function that potentially requires the miner to have an address.
	m := &Miner{
		cs:     cs,
		tpool:  tpool,
		wallet: w,

		blockMem:   make(map[types.BlockHeader]*types.Block),
		arbDataMem: make(map[types.BlockHeader][crypto.EntropySize]byte),
		headerMem:  make([]types.BlockHeader, HeaderMemory),

		persistDir: persistDir,
	}

	err := m.initPersist()
	if err != nil {
		return nil, errors.New("miner persistence startup failed: " + err.Error())
	}

	err = m.cs.ConsensusSetPersistentSubscribe(m, m.persist.RecentChange)
	if err == modules.ErrInvalidConsensusChangeID {
		// Perform a rescan of the consensus set if the change id is not found.
		// The id will only be not found if there has been desynchronization
		// between the miner and the consensus package.
		err = m.startupRescan()
		if err != nil {
			return nil, errors.New("miner startup failed - rescanning failed: " + err.Error())
		}
	} else if err != nil {
		return nil, errors.New("miner subscription failed: " + err.Error())
	}

	m.tpool.TransactionPoolSubscribe(m)

	// Save after synchronizing with consensus
	err = m.save()
	if err != nil {
		return nil, errors.New("miner could not save during startup: " + err.Error())
	}

	return m, nil
}

// Close terminates all ongoing processes involving the miner, enabling garbage
// collection.
func (m *Miner) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cs.Unsubscribe(m)

	var errs []error
	if err := m.save(); err != nil {
		errs = append(errs, fmt.Errorf("save failed: %v", err))
	}
	if err := m.log.Close(); err != nil {
		errs = append(errs, fmt.Errorf("log.Close failed: %v", err))
	}
	return build.JoinErrors(errs, "; ")
}

// checkAddress checks that the miner has an address, fetching an address from
// the wallet if not.
func (m *Miner) checkAddress() error {
	if m.persist.Address != (types.UnlockHash{}) {
		return nil
	}
	uc, err := m.wallet.NextAddress()
	if err != nil {
		return err
	}
	m.persist.Address = uc.UnlockHash()
	return nil
}

// BlocksMined returns the number of good blocks and stale blocks that have
// been mined by the miner.
func (m *Miner) BlocksMined() (goodBlocks, staleBlocks int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, blockID := range m.persist.BlocksFound {
		if m.cs.InCurrentPath(blockID) {
			goodBlocks++
		} else {
			staleBlocks++
		}
	}
	return
}
