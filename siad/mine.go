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

// Return true if currently mining, false otherwise.
func (e *Environment) Mining() bool {
	return e.mining
}

// ToggleMining creates a channel and mines until it receives a kill signal.
func (e *Environment) ToggleMining() (err error) {
	e.miningLock.Lock()
	defer e.miningLock.Unlock()

	if !e.mining {
		e.mining = true
		e.miningChan <- struct{}{}
	} else {
		e.mining = false
	}

	return
}

// Creates a block that is ready for nonce grinding.
func (e *Environment) blockForWork() (b *siacore.Block, target siacore.Target) {
	e.state.Lock()
	defer e.state.Unlock()

	// Fill out the block with potentially ready values.
	b = &siacore.Block{
		ParentBlockID: e.state.CurrentBlock().ID(),
		Timestamp:     siacore.Timestamp(time.Now().Unix()),
		MinerAddress:  e.CoinAddress(),
		Transactions:  e.state.TransactionPoolDump(),
	}
	b.MerkleRoot = b.TransactionMerkleRoot()
	target = e.state.CurrentTarget()

	// Fudge the timestamp if the block would otherwise be illegal.
	if b.Timestamp < e.state.EarliestLegalTimestamp() {
		b.Timestamp = e.state.EarliestLegalTimestamp()
	}

	return
}

// solveBlock() tries to find a solution by increasing the nonce and checking
// the hash repeatedly. Can fail.
func (e *Environment) solveBlock(b *siacore.Block, target siacore.Target) {
	for maxNonce := b.Nonce + IterationsPerAttempt; b.Nonce != maxNonce; b.Nonce++ {
		if b.CheckTarget(target) {
			e.processBlock(*b) // Block until the block has been processed.
			break
		}
	}

	// Ask for more work.
	e.miningChan <- struct{}{}
}

// mine attempts to generate blocks, and sends any found blocks down a channel.
func (e *Environment) mine() {
	for _ = range e.miningChan {
		e.miningLock.RLock()
		if e.mining {
			e.miningLock.RUnlock()
			b, target := e.blockForWork()
			go e.solveBlock(b, target)
		}
	}
}
