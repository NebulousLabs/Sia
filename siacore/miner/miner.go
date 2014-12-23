package miner

import (
	"math/rand"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
)

type Miner struct {
	// Block variables - helps the miner construct the next block.
	parent            consensus.BlockID
	transactions      []consensus.Transaction
	address           consensus.CoinAddress
	target            consensus.Target
	earliestTimestamp consensus.Timestamp

	desiredThreads       int // 0 if not mining.
	runningThreads       int
	iterationsPerAttempt uint64
	sync.RWMutex
}

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

// solveBlock tries to find a solution by increasing the nonce and checking
// the hash repeatedly. Can fail.
func (m *Miner) solveBlock(b *consensus.Block) (bool, error) {
	for maxNonce := b.Nonce + m.iterationsPerAttempt; b.Nonce != maxNonce; b.Nonce++ {
		if b.CheckTarget(m.target) {
			// Need to throw a block down a channel.
			/*
				err := m.processBlock(*b) // Block until the block has been processed.
			*/
			return true, nil
		}
	}

	return false, nil
}

func New(iterationsPerAttempt uint64) (m *Miner) {
	return &Miner{
		iterationsPerAttempt: iterationsPerAttempt,
	}
}

// TODO: Return useful info...
func (m *Miner) Info() ([]byte, error) {
	return nil, nil
}

func (m *Miner) Update(parent consensus.BlockID, transactions []consensus.Transaction, target consensus.Target, address consensus.CoinAddress, earliestTimestamp consensus.Timestamp) error {
	m.Lock()
	defer m.Unlock()

	m.parent = parent
	m.transactions = transactions
	m.target = target
	m.address = address
	m.earliestTimestamp = earliestTimestamp
	return nil
}

func (m *Miner) StartMining(threads int) error {
	m.Lock()
	defer m.Unlock()

	// Set max threads, then spin up as many miners as needed. The miners will
	// know to shut down if maxThreads decreases.
	m.desiredThreads = threads
	for i := m.runningThreads; i < m.desiredThreads; i++ {
		go m.mine()
	}

	return nil
}

func (m *Miner) StopMining() error {
	m.Lock()
	defer m.Unlock()

	// Set desiredThreads to 0. The miners will shut down automatically.
	m.desiredThreads = 0
	return nil
}

// mine attempts to generate blocks.
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
			m.solveBlock(m.blockForWork())
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
