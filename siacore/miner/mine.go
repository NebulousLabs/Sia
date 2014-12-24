package miner

import (
	"crypto/rand"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
)

// Creates a block that is ready for nonce grinding.
func (m *Miner) blockForWork() (b consensus.Block) {
	// Fill out the block with potentially ready values.
	b = consensus.Block{
		ParentBlockID: m.parent,
		Timestamp:     consensus.Timestamp(time.Now().Unix()),
		Nonce:         uint64(rand.Int()),
		MinerAddress:  m.address,
		Transactions:  m.transactions,
	}

	// If we've got a time earlier than the earliest legal timestamp, set the
	// timestamp equal to the earliest legal timestamp.
	if b.Timestamp < m.earliestTimestamp {
		b.Timestamp = m.earliestTimestamp

		// TODO: Add a single transaction that's just arbitrary data - a bunch
		// of randomly generated arbitrary data. This will provide entropy to
		// the block even though the timestamp isn't changing at all.
	}
	b.MerkleRoot = b.TransactionMerkleRoot()

	return
}

// mine attempts to generate blocks, and will run until desiredThreads is
// changd to be lower than `myThread`, which is set at the beginning of the
// function.
func (m *Miner) mine() {
	// Increment the number of threads running, because this thread is spinning
	// up. Also grab a number that will tell us when to shut down.
	m.Lock()
	m.runningThreads++
	myThread := m.runningThreads
	m.Unlock()

	// Try to solve a block repeatedly.
	for {
		// Grab the number of threads that are supposed to be running.
		m.RLock()
		desiredThreads := m.desiredThreads
		m.RUnlock()

		// If we are allowed to be running, mine a block, otherwise shut down.
		if desiredThreads >= myThread {
			m.SolveBlock()
		} else {
			m.Lock()
			// Need to check the mining status again, something might have
			// changed while waiting for the lock.
			if desiredThreads < myThread {
				m.runningThreads--
				m.Unlock()
				return
			}
			m.Unlock()
		}
	}
}

// solveBlock tries to find a solution by increasing the nonce and checking
// the hash repeatedly. Can fail.
func (m *Miner) SolveBlock() (b consensus.Block, solved bool, err error) {
	m.RLock()
	b = m.blockForWork()
	m.RUnlock()
	for maxNonce := b.Nonce + m.iterationsPerAttempt; b.Nonce != maxNonce; b.Nonce++ {
		if b.CheckTarget(m.target) {
			m.blockChan <- b
			solved = true
			return
		}
	}

	return
}

// StartMining spawns a bunch of mining threads which will mine until stop is
// called.
func (m *Miner) StartMining() error {
	m.Lock()
	defer m.Unlock()

	// Increase the number of threads to m.desiredThreads.
	m.desiredThreads = m.threads
	for i := m.runningThreads; i < m.desiredThreads; i++ {
		go m.mine()
	}

	return nil
}

// StopMining sets desiredThreads to 0, a value which is polled by mining
// threads. When set to 0, the mining threads will all cease mining.
func (m *Miner) StopMining() error {
	m.Lock()
	defer m.Unlock()

	// Set desiredThreads to 0. The miners will shut down automatically.
	m.desiredThreads = 0
	return nil
}
