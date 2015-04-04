package miner

import (
	"math/rand" // We should probably switch to crypto/rand, but we should use benchmarks first.

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/types"
)

// Creates a block that is ready for nonce grinding.
func (m *Miner) blockForWork() (b types.Block) {
	// Fill out the block with potentially ready values.
	b = types.Block{
		ParentID:     m.parent,
		Timestamp:    types.CurrentTimestamp(),
		Nonce:        uint64(rand.Int()),
		Transactions: m.transactions,
	}

	// Calculate the subsidy and create the miner payout.
	height, exists := m.state.HeightOfBlock(m.parent)
	if !exists {
		if build.DEBUG {
			panic("parent is not in state?")
		}
		return
	}
	subsidy := types.CalculateCoinbase(height + 1)
	for _, txn := range m.transactions {
		for _, fee := range txn.MinerFees {
			subsidy = subsidy.Add(fee)
		}
	}
	output := types.SiacoinOutput{Value: subsidy, UnlockHash: m.address}
	b.MinerPayouts = []types.SiacoinOutput{output}

	// If we've got a time earlier than the earliest legal timestamp, set the
	// timestamp equal to the earliest legal timestamp.
	if b.Timestamp < m.earliestTimestamp {
		b.Timestamp = m.earliestTimestamp
	}

	return
}

// mine attempts to generate blocks, and will run until desiredThreads is
// changd to be lower than `myThread`, which is set at the beginning of the
// function.
//
// The threading is fragile. Edit with caution!
func (m *Miner) threadedMine() {
	// Increment the number of threads running, because this thread is spinning
	// up. Also grab a number that will tell us when to shut down.
	m.mu.Lock()
	m.runningThreads++
	myThread := m.runningThreads
	m.mu.Unlock()

	// Try to solve a block repeatedly.
	for {
		// Grab the number of threads that are supposed to be running.
		m.mu.RLock()
		desiredThreads := m.desiredThreads
		m.mu.RUnlock()

		// If we are allowed to be running, mine a block, otherwise shut down.
		if desiredThreads >= myThread {
			// Grab the necessary variables for mining, and then attempt to
			// mine a block.
			m.mu.RLock()
			bfw := m.blockForWork()
			target := m.target
			iterations := m.iterationsPerAttempt
			m.mu.RUnlock()
			m.solveBlock(bfw, target, iterations)
		} else {
			m.mu.Lock()
			// Need to check the mining status again, something might have
			// changed while waiting for the lock.
			if desiredThreads < myThread {
				m.runningThreads--
				m.mu.Unlock()
				return
			}
			m.mu.Unlock()
		}
	}
}

// solveBlock takes a block, target, and number of iterations as input and
// tries to find a block that meets the target. This function can take a long
// time to complete, and should not be called with a lock.
func (m *Miner) solveBlock(blockForWork types.Block, target types.Target, iterations uint64) (b types.Block, solved bool, err error) {
	// solveBlock could operate on a pointer, but it's not strictly necessary
	// and it makes calling weirder/more opaque.
	b = blockForWork

	// Iterate through a bunch of nonces (from a random starting point) and try
	// to find a winnning solution.
	for maxNonce := b.Nonce + iterations; b.Nonce != maxNonce; b.Nonce++ {
		if b.CheckTarget(target) {
			err = m.state.AcceptBlock(b)
			if build.DEBUG {
				if err != nil {
					println(err.Error())
				}
			}
			m.gateway.RelayBlock(b)

			solved = true

			// Grab a new address for the miner.
			m.mu.Lock()
			var addr types.UnlockHash
			addr, _, err = m.wallet.CoinAddress()
			if err == nil { // Special case: only update the address if there was no error.
				m.address = addr
			}
			m.mu.Unlock()
			return
		}
	}

	return
}

// FindBlock will attempt to solve a block and add it to the state. While less
// efficient than StartMining, it is guaranteed to find at most one block.
func (m *Miner) FindBlock() (types.Block, bool, error) {
	m.mu.Lock()
	bfw := m.blockForWork()
	target := m.target
	iterations := m.iterationsPerAttempt
	m.mu.Unlock()

	return m.solveBlock(bfw, target, iterations)
}

// SolveBlock attempts to solve a block, returning the solved block without
// submitting it to the state.
func (m *Miner) SolveBlock(blockForWork types.Block, target types.Target) (b types.Block, solved bool) {
	m.mu.RLock()
	iterations := m.iterationsPerAttempt
	m.mu.RUnlock()

	// Iterate through a bunch of nonces (from a random starting point) and try
	// to find a winnning solution.
	b = blockForWork
	for maxNonce := b.Nonce + iterations; b.Nonce != maxNonce; b.Nonce++ {
		if b.CheckTarget(target) {
			solved = true
			return
		}
	}
	return
}

// StartMining spawns a bunch of mining threads which will mine until stop is
// called.
func (m *Miner) StartMining() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Increase the number of threads to m.desiredThreads.
	m.desiredThreads = m.threads
	for i := m.runningThreads; i < m.desiredThreads; i++ {
		go m.threadedMine()
	}

	return nil
}

// StopMining sets desiredThreads to 0, a value which is polled by mining
// threads. When set to 0, the mining threads will all cease mining.
func (m *Miner) StopMining() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set desiredThreads to 0. The miners will shut down automatically.
	m.desiredThreads = 0
	return nil
}
