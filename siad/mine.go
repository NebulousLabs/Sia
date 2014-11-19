package siad

import (
	"errors"
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
}

// Creates a block that is ready for nonce grinding.
func (m *Miner) blockForWork(minerAddress siacore.CoinAddress) (b *siacore.Block, target siacore.Target) {
	b = &siacore.Block{
		ParentBlockID: m.state.CurrentBlockID,
		Timestamp:     siacore.Timestamp(time.Now().Unix()),
		MinerAddress:  minerAddress,
		Transactions:  m.state.TransactionPoolDump(),
	}
	// Fudge the timestamp if the block would otherwise be illegal.
	if b.Timestamp < m.state.CurrentBlockNode().EarliestLegalChildTimestamp() {
		b.Timestamp = m.state.CurrentBlockNode().EarliestLegalChildTimestamp()
	}

	// Add the transactions from the transaction pool.
	b.MerkleRoot = b.ExpectedTransactionMerkleRoot()

	// Determine the target for the block.
	target = m.state.CurrentBlockNode().Target

	return
}

// solveBlock() tries to find a solution by increasing the nonce and checking
// the hash repeatedly. Can fail.
func solveBlock(b *siacore.Block, target siacore.Target) bool {
	for i := 0; i < IterationsPerAttempt; i++ {
		if b.CheckTarget(target) {
			return true
		}

		b.Nonce++
	}

	return false
}

// attemptToGenerateBlock attempts to generate a block, but instead of running
// until a block is found, it just tries a single time.
func (m *Miner) attemptToGenerateBlock(minerAddress siacore.CoinAddress) (b *siacore.Block, err error) {
	m.state.Lock()
	b, target := m.blockForWork(minerAddress)
	m.state.Unlock()

	if solveBlock(b, target) {
		return
	} else {
		err = errors.New("could not find block")
		return
	}
}

// generateBlock() creates a new block, will keep working until a block is
// found, which may take a long time.
func (m *Miner) generateBlock(minerAddress siacore.CoinAddress) (b *siacore.Block) {
	for {
		var err error
		b, err = m.attemptToGenerateBlock(minerAddress)
		if err == nil {
			return b
		}
	}
}

// ToggleMining creates a channel and mines until it receives a kill signal.
func (m *Miner) ToggleMining(minerAddress siacore.CoinAddress) {
	if !m.mining {
		m.killMining = make(chan struct{})
		m.mining = true
	}

	// Need some channel to wait on to kill the function.
	for {
		select {
		case <-m.killMining:
			return

		default:
			block, err := m.attemptToGenerateBlock(minerAddress)
			if err == nil {
				m.state.AcceptBlock(*block)
			}
		}
	}
}
