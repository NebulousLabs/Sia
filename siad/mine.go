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
	blockChan  chan *siacore.Block
	killMining chan struct{}

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
	if b.Timestamp < m.state.CurrentEarliestLegalTimestamp() {
		b.Timestamp = m.state.CurrentEarliestLegalTimestamp()
	}

	// Add the transactions from the transaction pool.
	b.MerkleRoot = b.ExpectedTransactionMerkleRoot()

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
				m.blockChan <- b
			}
		}
	}
}

// CreateMiner takes an address as input and returns a miner. All blocks mined
// by the miner will have the subsidies sent to the subsidyAddress.
func CreateMiner(subsidyAddress siacore.CoinAddress) *Miner {
	m := new(Miner)
	m.killMining = make(chan struct{})
	m.blockChan = make(chan *siacore.Block, 10)
	m.subsidyAddress = subsidyAddress
	return m
}

// ToggleMining creates a channel and mines until it receives a kill signal.
func (e *Environment) ToggleMining() {
	if !e.miner.mining {
		e.miner.mining = true
		go e.miner.mine()
	} else {
		e.miner.mining = false
		e.miner.killMining <- struct{}{}
	}
}
