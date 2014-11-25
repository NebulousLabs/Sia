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
	mining     bool
	BlockChan  chan *siacore.Block
	killMining chan struct{}
}

func (m *Miner) Mining() bool {
	return m.mining
}

// Creates a block that is ready for nonce grinding.
func (m *Miner) blockForWork(state *siacore.State, minerAddress siacore.CoinAddress) (b *siacore.Block, target siacore.Target) {
	state.Lock()
	defer state.Unlock()
	b = &siacore.Block{
		ParentBlockID: state.CurrentBlockID,
		Timestamp:     siacore.Timestamp(time.Now().Unix()),
		MinerAddress:  minerAddress,
		Transactions:  state.TransactionPoolDump(),
	}
	// Fudge the timestamp if the block would otherwise be illegal.
	if b.Timestamp < state.CurrentEarliestLegalTimestamp() {
		b.Timestamp = state.CurrentEarliestLegalTimestamp()
	}

	// Add the transactions from the transaction pool.
	b.MerkleRoot = b.ExpectedTransactionMerkleRoot()

	// Determine the target for the block.
	target = state.CurrentTarget()

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

// ToggleMining creates a channel and mines until it receives a kill signal.
func (m *Miner) ToggleMining(state *siacore.State, minerAddress siacore.CoinAddress) {
	if !m.mining {
		m.mining = true
		go m.mine(state, minerAddress)
	} else {
		m.mining = false
		m.killMining <- struct{}{}
	}
}

// mine attempts to generate blocks, and sends any found blocks down a channel.
func (m *Miner) mine(state *siacore.State, minerAddress siacore.CoinAddress) {
	for {
		select {
		case <-m.killMining:
			return

		default:
			b, target := m.blockForWork(state, minerAddress)
			if solveBlock(b, target) {
				m.BlockChan <- b
			}
		}
	}
}

// generateBlock() creates a new block, will keep working until a block is
// found, which may take a long time.
func (m *Miner) generateBlock(state *siacore.State, minerAddress siacore.CoinAddress) (b *siacore.Block) {
	if !m.mining {
		m.mining = true
		go m.mine(state, minerAddress)
	}
	b = <-m.BlockChan
	m.killMining <- struct{}{}
	return b
}

func CreateMiner() *Miner {
	m := new(Miner)
	m.killMining = make(chan struct{})
	m.BlockChan = make(chan *siacore.Block, 10)
	return m
}
