package miner

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// Creates a block ready for nonce grinding, also returning the MerkleRoot of
// the block. Getting the MerkleRoot of a block requires encoding and hashing
// in a specific way, which are implementation details we didn't want to
// require external miners to need to worry about. All blocks returned are
// unique, which means all miners can safely start at the '0' nonce.
func (m *Miner) blockForWork() (types.Block, crypto.Hash, types.Target) {
	// Fill out the block with potentially ready values.
	b := types.Block{
		ParentID:  m.parent,
		Timestamp: types.CurrentTimestamp(),
	}

	// Add a transaction with random arbitrary data so that all blocks returned
	// by this function are unique - this means that miners can safely start at
	// the 0 nonce.
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	b.Transactions = append(m.transactions, types.Transaction{
		ArbitraryData: []string{"NonSia" + string(randBytes)},
	})

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

	return b, b.MerkleRoot(), m.target
}

// submitBlock takes a solved block and submits it to the blockchain.
// submitBlock should not be called with a lock.
func (m *Miner) submitBlock(b types.Block) error {
	// Give the block to the consensus set.
	err := m.cs.AcceptBlock(b)
	if err != nil {
		m.mu.Lock()
		m.tpool.PurgeTransactionPool()
		m.mu.Unlock()
		return err
	}
	if build.Release != "testing" {
		fmt.Println("Found a block! Reward will be received in 50 blocks.")
	}

	// Grab a new address for the miner.
	m.mu.Lock()
	m.blocksFound = append(m.blocksFound, b.ID())
	var addr types.UnlockHash
	addr, _, err = m.wallet.CoinAddress(false) // false indicates that the address should not be visible to the user.
	if err == nil {                            // Special case: only update the address if there was no error.
		m.address = addr
	}
	m.mu.Unlock()
	return err
}

// solveBlock takes a block, target, and number of iterations as input and
// tries to find a block that meets the target. This function can take a long
// time to complete, and should not be called with a lock.
func (m *Miner) solveBlock(blockForWork types.Block, blockMerkleRoot crypto.Hash, target types.Target) (b types.Block, solved bool, err error) {
	b = blockForWork
	hashbytes := make([]byte, 72)
	copy(hashbytes, b.ParentID[:])
	copy(hashbytes[40:], blockMerkleRoot[:])

	nonce := (*uint64)(unsafe.Pointer(&hashbytes[32]))
	*nonce = b.Nonce
	for i := 0; i < iterationsPerAttempt; i++ {
		id := crypto.HashBytes(hashbytes)
		if bytes.Compare(target[:], id[:]) >= 0 {
			b.Nonce = binary.LittleEndian.Uint64(hashbytes[32:40])
			err = m.submitBlock(b)
			if err != nil {
				return
			}
			solved = true
			return
		}
		*nonce++
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
		m.hashRate = int64((m.attempts * iterationsPerAttempt * 1e9)) / (time.Now().UnixNano() - m.startTime.UnixNano())
		m.startTime = time.Now()
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
			bfw, blockMerkleRoot, target := m.blockForWork()
			m.increaseAttempts()
			m.mu.Unlock()
			m.solveBlock(bfw, blockMerkleRoot, target)
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

// BlockForWork returns a block that is ready for nonce grinding, along with
// the root hash of the block.
func (m *Miner) BlockForWork() (types.Block, crypto.Hash, types.Target) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blockForWork()
}

// FindBlock will attempt to solve a block and add it to the state. While less
// efficient than StartMining, it is guaranteed to find at most one block.
func (m *Miner) FindBlock() (types.Block, bool, error) {
	m.mu.Lock()
	bfw, blockMerkleRoot, target := m.blockForWork()
	m.mu.Unlock()

	return m.solveBlock(bfw, blockMerkleRoot, target)
}

// SolveBlock attempts to solve a block, returning the solved block without
// submitting it to the state. This function is primarily to help with testing,
// and is very slow.
func (m *Miner) SolveBlock(blockForWork types.Block, target types.Target) (b types.Block, solved bool) {
	b = blockForWork
	for b.Nonce = 0; b.Nonce < iterationsPerAttempt; b.Nonce++ {
		if b.CheckTarget(target) {
			solved = true
			return
		}
	}
	return
}

// SubmitBlock accepts a block with a valid target and presents it to the
// consensus set.
func (m *Miner) SubmitBlock(b types.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.submitBlock(b)
}
