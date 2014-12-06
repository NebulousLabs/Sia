package siad

import (
	"time"

	"github.com/NebulousLabs/Andromeda/siacore"
)

const (
	// If it takes less than 1 second to go through all of the iterations,
	// then repeat work will be performed.
	IterationsPerAttempt = 10 * 1000 * 1000
)

type Miner struct {
	state *siacore.State

	mining     bool
	killMining chan struct{}
	blockChan  chan<- siacore.Block

	subsidyAddress siacore.CoinAddress
}

func (m *Miner) Mining() bool {
	return m.mining
}

// Creates a block that is ready for nonce grinding.
func (m *Miner) blockForWork() (b *siacore.Block, target siacore.Target) {
	m.state.Lock()
	defer m.state.Unlock()

	b = &siacore.Block{
		ParentBlockID: m.state.CurrentBlock().ID(),
		Timestamp:     siacore.Timestamp(time.Now().Unix()),
		MinerAddress:  m.subsidyAddress,
		Transactions:  m.state.TransactionPoolDump(),
	}

	// Fudge the timestamp if the block would otherwise be illegal.
	if b.Timestamp < m.state.EarliestLegalTimestamp() {
		b.Timestamp = m.state.EarliestLegalTimestamp()
	}

	// Add the transactions from the transaction pool.
	b.MerkleRoot = b.TransactionMerkleRoot()

	// Determine the target for the block.
	target = m.state.CurrentTarget()

	return
}

// solveBlock() tries to find a solution by increasing the nonce and checking
// the hash repeatedly. Can fail.
func solveBlock(b *siacore.Block, target siacore.Target) bool {
	maxNonce := b.Nonce + IterationsPerAttempt
	for ; b.Nonce < maxNonce; b.Nonce++ {
		if b.CheckTarget(target) {
			return true
		}
	}

	return false
}

// mine attempts to generate blocks, and sends any found blocks down a channel.
func (m *Miner) mine() {
	for {
		select {
		case <-m.killMining:
			return

		default:
			b, target := m.blockForWork()
			if solveBlock(b, target) {
				m.blockChan <- *b
			}
		}
	}
}

// CreateMiner takes an address as input and returns a miner. All blocks mined
// by the miner will have the subsidies sent to the subsidyAddress.
func CreateMiner(s *siacore.State, blockChan chan<- siacore.Block, subsidyAddress siacore.CoinAddress) *Miner {
	return &Miner{
		state:          s,
		killMining:     make(chan struct{}),
		blockChan:      blockChan,
		subsidyAddress: subsidyAddress,
	}
}

// A getter for the mining variable of the miner.
func (e *Environment) Mining() bool {
	return e.miner.mining
}

// ToggleMining creates a channel and mines until it receives a kill signal.
func (e *Environment) ToggleMining() (err error) {
	if !e.miner.mining {
		e.miner.mining = true
		go e.miner.mine()
	} else {
		e.miner.mining = false
		e.miner.killMining <- struct{}{}
	}

	return
}
