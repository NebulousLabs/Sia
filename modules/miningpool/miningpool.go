package miningpool

import (
	"errors"
	"log"
	//"math/big"
	"net"

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

	// A list of blocks that have been submitted to the network. netPayout is
	// the total amount of money sent to miners, while profit is the amount of
	// money earned
	blocksFound []types.BlockID
	netPayout   types.Currency
	profit      types.Currency

	// Some variables to keep track of each miner's payment channel
	// For now, the payment channels are a map of miner addresses to channels. The problem with this is that if miners want to change to a new address (e.g. avoid address reuse), they have to create a new payment channel
	channels map[types.UnlockHash]paymentChannel

	// Subscription management variables.
	subscribers []chan struct{}

	myAddr   modules.NetAddress
	listener net.Listener

	modules.MiningPoolSettings

	persistDir string
	log        *log.Logger
	mu         *sync.RWMutex
}

// New returns a ready-to-go miningpool
func New(cs modules.ConsensusSet, tpool modules.TransactionPool, w modules.Wallet, addr string, persistDir string) (*MiningPool, error) {
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

		MiningPoolSettings: modules.MiningPoolSettings{
			MaxConnections: 1024,
			TargetMultiple: 255,
			//MiningPoolCut:  *big.NewRat(5, 100), // 0.05
			//MinerCut:       *big.NewRat(3, 100), // 0.03
		},

		persistDir: persistDir,

		mu: sync.New(modules.SafeMutexDelay, 1),
	}
	err := mp.initPersist()
	if err != nil {
		return nil, err
	}

	// Create listener and set address
	mp.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	_, port, _ := net.SplitHostPort(mp.listener.Addr().String())
	mp.myAddr = modules.NetAddress(net.JoinHostPort("::1", port))

	// Learn our external IP.
	//go mp.learnHostName()

	// Forward the hosting port, if possible
	//go mp.forwardPort(port)

	// spawn listener
	go mp.listen()

	// mp.tpool.TransactionPoolSubscribe(mp) ?
	return mp, nil
}

func (mp *MiningPool) Settings() modules.MiningPoolSettings {
	lockID := mp.mu.RLock()
	defer mp.mu.RUnlock(lockID)
	return mp.MiningPoolSettings
}
