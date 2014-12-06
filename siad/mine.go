package main

import (
	"math/rand"
	"time"

	"github.com/NebulousLabs/Andromeda/siacore"
)

var (
	IterationsPerAttempt uint64 = 10 * 1000 * 1000
	MiningThreads        int    = 1
)

// Return true if currently mining, false otherwise.
func (e *Environment) Mining() bool {
	e.miningLock.RLock()
	defer e.miningLock.RUnlock()
	return e.mining
}

// ToggleMining creates a channel and mines until it receives a kill signal.
func (e *Environment) ToggleMining() (err error) {
	e.miningLock.Lock()
	defer e.miningLock.Unlock()

	if !e.mining {
		e.mining = true
		for i := e.miningThreads; i < MiningThreads; i++ {
			go e.mine()
		}
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
		Nonce:         uint64(rand.Int()),
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
func (e *Environment) solveBlock(b *siacore.Block, target siacore.Target) bool {
	for maxNonce := b.Nonce + IterationsPerAttempt; b.Nonce != maxNonce; b.Nonce++ {
		if b.CheckTarget(target) {
			e.processBlock(*b) // Block until the block has been processed.
			return true
		}
	}

	return false
}

// mine attempts to generate blocks, and sends any found blocks down a channel.
func (e *Environment) mine() {
	e.miningLock.Lock()
	e.miningThreads++
	e.miningLock.Unlock()

	// Try to solve a block repeatedly.
	for {
		// Get the mining status before trying to work.
		e.miningLock.RLock()
		mining := e.mining
		e.miningLock.RUnlock()

		// If we are still mining, do some work, otherwise disable mining and
		// decrease the thread count for miners.
		if mining {
			b, target := e.blockForWork()
			e.solveBlock(b, target)
		} else {
			e.miningLock.Lock()

			// Need to check the mining status again, something might have
			// changed while waiting for the lock.
			if !e.mining {
				e.miningThreads--
				e.miningLock.Unlock()
				return
			}
			e.miningLock.Unlock()
		}
	}
}
