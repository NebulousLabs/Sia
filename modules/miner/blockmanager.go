package miner

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// Creates a block ready for nonce grinding, also returning the MerkleRoot of
// the block. Getting the MerkleRoot of a block requires encoding and hashing
// in a specific way, which are implementation details we didn't want to
// require external miners to need to worry about. All blocks returned are
// unique, which means all miners can safely start at the '0' nonce.
func (m *Miner) blockForWork() (types.Block, types.Target) {
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
	return b, m.target
}

// BlockForWork returns a block that is ready for nonce grinding, along with
// the root hash of the block.
func (m *Miner) BlockForWork() (b types.Block, merkleRoot crypto.Hash, t types.Target) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, t = m.blockForWork()
	merkleRoot = b.MerkleRoot()
	return b, merkleRoot, t
}

// BlockForWork returns a block that is ready for nonce grinding, along with
// the root hash of the block.
func (m *Miner) HeaderForWork() (types.BlockHeader, types.Target) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Grab a block for work.
	block, target := m.blockForWork()

	// Save a mapping between the block and its header, replacing the block
	// that was stored 'headerForWorkMemory' requests ago.
	delete(m.blockMem, m.headerMem[m.memProgress])
	m.blockMem[block.Header()] = block
	m.headerMem[m.memProgress] = block.Header()
	m.memProgress++
	if m.memProgress == headerForWorkMemory {
		m.memProgress = 0
	}

	// Return the header and target.
	return block.Header(), target
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

// submitBlock takes a solved block and submits it to the blockchain.
// submitBlock should not be called with a lock.
func (m *Miner) SubmitHeader(bh types.BlockHeader) error {
	// Fetch the block from the blockMem.
	var zeroNonce [8]byte
	lookupBH := bh
	lookupBH.Nonce = zeroNonce
	m.mu.Lock()
	b, exists := m.blockMem[lookupBH]
	m.mu.Unlock()
	if !exists {
		fmt.Println("Block returned late - too many calls to the miner.")
		return errors.New("block header returned late - block was cleared from memory")
	}
	b.Nonce = bh.Nonce
	return m.SubmitBlock(b)
}
