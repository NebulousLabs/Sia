package miner

import (
	"bytes"
	"encoding/binary"
	"math/rand" // We should probably switch to crypto/rand, but we should use benchmarks first.
	"time"
	"unsafe"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
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
	subsidy := types.CalculateCoinbase(m.height + 1)
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

// solveBlock takes a block, target, and number of iterations as input and
// tries to find a block that meets the target. This function can take a long
// time to complete, and should not be called with a lock.
func (m *Miner) solveBlock(blockForWork types.Block, target types.Target, iterations uint64) (b types.Block, solved bool, err error) {
	b = blockForWork
	bRoot := b.MerkleRoot()
	hashbytes := make([]byte, 72)
	copy(hashbytes, b.ParentID[:])
	copy(hashbytes[40:], bRoot[:])

	// Iterate through a bunch of nonces (from a random starting point) and try
	// to find a winnning solution.
	nonce := (*uint64)(unsafe.Pointer(&hashbytes[32]))
	*nonce = b.Nonce
	for i := 0; i < int(iterations); i++ {
		*nonce++
		id := crypto.HashBytes(hashbytes)
		if bytes.Compare(target[:], id[:]) >= 0 {
			b.Nonce = binary.LittleEndian.Uint64(hashbytes[32:])
			err = m.cs.AcceptBlock(b)
			if err != nil {
				println("Mined a bad block " + err.Error())
				m.tpool.PurgeTransactionPool()
			}
			solved = true
			if build.Release != "testing" {
				println("Found a block! Reward will be received in 50 blocks.")
			}

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

// increaseAttempts is the miner's way of guaging it's own hashrate. After it's
// made 100 attempts to find a block, it calculates a hashrate based on how
// much time has passed. The number of attempts in progress is set to 0
// whenever mining starts or stops, which prevents weird low values from
// cropping up.
func (m *Miner) increaseAttempts() {
	m.attempts++
	if m.attempts >= 100 {
		m.hashRate = int64((m.attempts * m.iterationsPerAttempt * 1e9)) / (time.Now().UnixNano() - m.startTime)
		m.startTime = time.Now().UnixNano()
		m.attempts = 0
	}
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
		m.mu.Lock()
		desiredThreads := m.desiredThreads
		m.mu.Unlock()

		// If we are allowed to be running, mine a block, otherwise shut down.
		if desiredThreads >= myThread {
			// Grab the necessary variables for mining, and then attempt to
			// mine a block.
			m.mu.Lock()
			bfw := m.blockForWork()
			target := m.target
			iterations := m.iterationsPerAttempt
			m.increaseAttempts()
			m.mu.Unlock()
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
	m.mu.Lock()
	iterations := m.iterationsPerAttempt
	m.mu.Unlock()

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
