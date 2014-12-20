package siacore

// TODO: Miner is probably creating wayyyyy toooo many addresses when it calls
// blockForWork() - most of these addresses will never get outputs.

import (
	"math/rand"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
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
func (e *Environment) StartMining() {
	e.miningLock.Lock()
	defer e.miningLock.Unlock()
	e.mining = true
	for i := e.miningThreads; i < MiningThreads; i++ {
		go e.mine()
	}
}

func (e *Environment) StopMining() {
	e.miningLock.Lock()
	defer e.miningLock.Unlock()
	e.mining = false
}

// Creates a block that is ready for nonce grinding.
func (e *Environment) blockForWork() (b *consensus.Block, target consensus.Target) {
	e.state.RLock()
	defer e.state.RUnlock()

	// Fill out the block with potentially ready values.
	b = &consensus.Block{
		ParentBlockID: e.state.CurrentBlock().ID(),
		Timestamp:     consensus.Timestamp(time.Now().Unix()),
		Nonce:         uint64(rand.Int()),
		Transactions:  e.state.TransactionPoolDump(),
	}

	// Get the address for the miner.
	var err error
	b.MinerAddress, err = e.CoinAddress()
	if err != nil {
		panic(err) // TODO: something about this panic.
	}
	b.MerkleRoot = b.TransactionMerkleRoot()
	target = e.state.CurrentTarget()

	// Fudge the timestamp if the block would otherwise be illegal.
	// TODO: this is unsafe
	if b.Timestamp < e.state.EarliestLegalTimestamp() {
		b.Timestamp = e.state.EarliestLegalTimestamp()
	}

	return
}

// solveBlock tries to find a solution by increasing the nonce and checking
// the hash repeatedly. Can fail.
func (e *Environment) solveBlock(b *consensus.Block, target consensus.Target) bool {
	for maxNonce := b.Nonce + IterationsPerAttempt; b.Nonce != maxNonce; b.Nonce++ {
		if b.CheckTarget(target) {
			e.processBlock(*b) // Block until the block has been processed.
			return true
		}
	}

	return false
}

// mine attempts to generate blocks.
func (e *Environment) mine() {
	e.miningLock.Lock()
	e.miningThreads++
	e.miningLock.Unlock()
	// Try to solve a block repeatedly.
	for {
		// If we are still mining, do some work, otherwise disable mining and
		// decrease the thread count for miners.
		if e.Mining() {
			e.solveBlock(e.blockForWork())
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
