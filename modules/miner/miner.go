package miner

import (
	"errors"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// HeaderMemory is the number of previous calls to 'header'
	// that are remembered. Additionally, 'header' will only poll for a
	// new block every 'headerMemory / blockMemory' times it is
	// called. This reduces the amount of memory used, but comes at the cost of
	// not always having the most recent transactions.
	HeaderMemory = 10000

	// BlockMemory is the maximum number of blocks the miner will store
	// Blocks take up to 2 megabytes of memory, which is why this number is
	// limited.
	BlockMemory = 50

	// MaxSourceBlockAge is the maximum amount of time that is allowed to
	// elapse between generating source blocks.
	MaxSourceBlockAge = 60 * time.Second
)

var (
	errNilCS     = errors.New("miner cannot use a nil consensus set")
	errNilTpool  = errors.New("miner cannot use a nil transaction pool")
	errNilWallet = errors.New("miner cannot use a nil wallet")
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
	persistDir string
	persist    persistence
	log        *persist.FileLogger
	mu         sync.RWMutex
}

// newHandleErrInvalidConsensusChangeID manages the rescanning in the event
// that a subscription during startup fails.
func (m *Miner) newHandleErrInvalidConsensusChangeID() error {
	// If the change id is not recognized, it means that the consensus set
	// has somehow reset or otherwise changed, and a rescan must be
	// performed.
	m.log.Println("Inconsistency found between the miner and the consensus set, fixing...")

	// Perform the rescan and block until rescanning is complete. Rescan
	// does need access to a lock, but no lock is held during startup
	// anyway, so blocking until the channel returns an error is safe.
	c := make(chan error)
	m.threadedConsensusRescan(c)
	return <-c
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
		err = m.newHandleErrInvalidConsensusChangeID()
		if err != nil {
			return nil, errors.New("miner startup failed - rescanning failed: " + err.Error())
		}
	} else if err != nil {
		return nil, errors.New("miner subscription failed: " + err.Error())
	}

	m.tpool.TransactionPoolSubscribe(m)
	return m, nil
}

// Close terminates all portions of the
func (m *Miner) Close() error {
	m.cs.Unsubscribe(m)
	return m.log.Close()
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
