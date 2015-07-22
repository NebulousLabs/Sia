package miningpool

import (
	"errors"
	"log"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// Will only support maxConnections miners
	maxConnections = 1024

	// How much easier a partial block is than a block
	// pool target is target * targetMultiple
	targetMultiple = 255

	// Percent of block payout that the pool keeps
	miningPoolCut = 0.05

	// Percent of block payout that the miner who mined the block gets to keep
	// This encourages miners to submit full blocks instead of throwing them away
	minerCut = 0.03
)

type MiningPool struct {
	// Module dependencies
	cs     modules.ConsensusSet
	tpool  modules.TransactionPool
	wallet modules.Wallet

	// List of headers whose miner has already been paid (prevent double submits)
	spentHeaders []types.Block // Should this be a map?

	// A list of blocks that have been submitted to the network
	blocksFound []types.BlockID

	// Some variables to keep track of payment channels?
	// I'll figure this out once I start implementing payment channel functionality

	// Subscription management variables.
	subscribers []chan struct{}

	persistDir string
	log        *log.Logger
	mu         *sync.RWMutex
}

// New returns a ready-to-go miningpool
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, w modules.Wallet, persistDir string) (*MiningPool, error) {
	// Creates the mining pool and its dependencies
	if cs == nil {
		return nil, errors.New("mining pool cannot use a nil state")
	}
	if tpool == nil {
		return nil, errors.New("mining pool cannot use a nil transaction pool")
	}
	if w == nil {
		return nil, errors.New("mining pool cannot use a nil wallet")
	}

	mp := &MiningPool{
		cs:     cs,
		tpool:  tpool,
		wallet: w,

		persistDir: persistDir,
		mu:         sync.New(modules.SafeMutexDelay, 1),
	}
	err := mp.initPersist()
	if err != nil {
		return nil, err
	}
	// mp.tpool.TransactionPoolSubscribe(mp) ?
	return mp, nil
}
