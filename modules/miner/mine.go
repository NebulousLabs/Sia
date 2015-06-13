package miner

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// Creates a block ready for nonce grinding, also returning the MerkleRoot of
// the block. Getting the MerkleRoot of a block requires encoding and hashing
// in a specific way, which are implementation details we didn't want to
// require external miners to need to worry about. All blocks returned are
// unique, which means all miners can safely start at the '0' nonce.
func (m *Miner) blockForWork() (types.Block, crypto.Hash, types.Target) {
	// Determine the timestamp.
	blockTimestamp := types.CurrentTimestamp()
	if blockTimestamp < m.earliestTimestamp {
		blockTimestamp = m.earliestTimestamp
	}

	// Create the miner payouts.
	subsidy := types.CalculateCoinbase(m.height)
	for _, txn := range m.transactions {
		for _, fee := range txn.MinerFees {
			subsidy = subsidy.Add(fee)
		}
	}
	blockPayouts := []types.SiacoinOutput{types.SiacoinOutput{Value: subsidy, UnlockHash: m.address}}

	// Create the list of transacitons, including the randomized transaction.
	// The transactions are assembled by calling append(singleElem,
	// existingSlic) because doing it the reverse way has some side effects,
	// creating a race condition and ultimately changing the block hash for
	// other parts of the program. This is related to the fact that slices are
	// pointers, and not immutable objects. Use of the builtin `copy` function
	// when passing objects like blocks around may fix this problem.
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	randTxn := types.Transaction{
		ArbitraryData: []string{"NonSia" + string(randBytes)},
	}
	blockTransactions := append([]types.Transaction{randTxn}, m.transactions...)

	// Assemble the block
	b := types.Block{
		ParentID:     m.parent,
		Timestamp:    blockTimestamp,
		MinerPayouts: blockPayouts,
		Transactions: blockTransactions,
	}

	return b, b.MerkleRoot(), m.target
}

// submitBlock takes a solved block and submits it to the blockchain.
// submitBlock should not be called with a lock.
func (m *Miner) SubmitBlock(b types.Block) error {
	// Give the block to the consensus set.
	err := m.cs.AcceptBlock(b)
	if err != nil {
		m.mu.Lock()
		m.tpool.PurgeTransactionPool()
		m.mu.Unlock()
		fmt.Println("Error: an invalid block was submitted:", err)
		return err
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
	hashbytes := make([]byte, 80)
	copy(hashbytes, b.ParentID[:])
	binary.LittleEndian.PutUint64(hashbytes[40:48], uint64(b.Timestamp))
	copy(hashbytes[48:], blockMerkleRoot[:])

	nonce := (*uint64)(unsafe.Pointer(&hashbytes[32]))
	for i := 0; i < iterationsPerAttempt; i++ {
		id := crypto.HashBytes(hashbytes)
		if bytes.Compare(target[:], id[:]) >= 0 {
			copy(b.Nonce[:], hashbytes[32:40])
			err = m.SubmitBlock(b)
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
	blockMerkleRoot := b.MerkleRoot()
	hashbytes := make([]byte, 80)
	copy(hashbytes, b.ParentID[:])
	binary.LittleEndian.PutUint64(hashbytes[40:48], uint64(b.Timestamp))
	copy(hashbytes[48:], blockMerkleRoot[:])

	nonce := (*uint64)(unsafe.Pointer(&hashbytes[32]))
	for i := 0; i < iterationsPerAttempt; i++ {
		id := crypto.HashBytes(hashbytes)
		if bytes.Compare(target[:], id[:]) >= 0 {
			copy(b.Nonce[:], hashbytes[32:40])
			return b, true
		}
		*nonce++
	}
	return b, false
}
