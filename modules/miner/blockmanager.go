package miner

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
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
	randBytes, _ := crypto.RandBytes(types.SpecifierLen)
	randTxn := types.Transaction{
		ArbitraryData: [][]byte{append(modules.PrefixNonSia[:], randBytes...)},
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
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)

	b, t = m.blockForWork()
	merkleRoot = b.MerkleRoot()
	return b, merkleRoot, t
}

// BlockForWork returns a block that is ready for nonce grinding, along with
// the root hash of the block.
func (m *Miner) HeaderForWork() (types.BlockHeader, types.Target) {
	lockID := m.mu.Lock()
	defer m.mu.Unlock(lockID)

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
		m.tpool.PurgeTransactionPool()
		m.log.Println("ERROR: an invalid block was submitted:", err)
		return err
	}

	// Grab a new address for the miner.
	lockID := m.mu.Lock()
	m.blocksFound = append(m.blocksFound, b.ID())
	var addr types.UnlockHash
	addr, _, err = m.wallet.CoinAddress(false) // false indicates that the address should not be visible to the user.
	if err == nil {                            // Special case: only update the address if there was no error.
		m.address = addr
	}
	m.mu.Unlock(lockID)
	return err
}

// submitBlock takes a solved block and submits it to the blockchain.
// submitBlock should not be called with a lock.
func (m *Miner) SubmitHeader(bh types.BlockHeader) error {
	// Fetch the block from the blockMem.
	var zeroNonce [8]byte
	lookupBH := bh
	lookupBH.Nonce = zeroNonce
	lockID := m.mu.Lock()
	b, exists := m.blockMem[lookupBH]
	m.mu.Unlock(lockID)
	if !exists {
		err := errors.New("block header returned late - block was cleared from memory")
		m.log.Println("ERROR:", err)
		return err
	}
	b.Nonce = bh.Nonce
	return m.SubmitBlock(b)
}
