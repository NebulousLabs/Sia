package miner

import (
	"errors"
	"time"

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
	// Update the timestmap.
	if m.unsolvedBlock.Timestamp < types.CurrentTimestamp() {
		m.unsolvedBlock.Timestamp = types.CurrentTimestamp()
	}

	// Update the address + payouts.
	_ = m.checkAddress() // Err is ignored - address generation failed but can't do anything about it (log maybe).
	m.unsolvedBlock.MinerPayouts = []types.SiacoinOutput{types.SiacoinOutput{Value: m.unsolvedBlock.CalculateSubsidy(m.height + 1), UnlockHash: m.address}}

	// TODO: DEPRECATED
	//
	// Add an arb-data txn to the block.
	randBytes, _ := crypto.RandBytes(types.SpecifierLen)
	randTxn := types.Transaction{
		ArbitraryData: [][]byte{append(modules.PrefixNonSia[:], randBytes...)},
	}
	m.unsolvedBlock.Transactions = append([]types.Transaction{randTxn}, m.unsolvedBlock.Transactions...)

	return m.unsolvedBlock, m.target
}

// prepareNewBlock sets the blockmanager up to generate a new block next time
// HeaderForWork is called. Note that calling this may diminish from the max
// number of headers that can be stored (because memProgress gets shifted forward)
func (m *Miner) prepareNewBlock() {
	// Move mem progress forward. This prevents more than blockForWorkMemory
	// blocks from being created in the case of a slow miner. We also have
	// to delete all headers as we go to ensure old blocks get removed from memory
	for m.memProgress%(headerForWorkMemory/blockForWorkMemory) != 0 {
		delete(m.blockMem, m.headerMem[m.memProgress])
		delete(m.arbDataMem, m.headerMem[m.memProgress])
		m.memProgress++
		if m.memProgress == headerForWorkMemory {
			m.memProgress = 0
		}
	}
}

// HeaderForWork returns a block that is ready for nonce grinding, along with
// the root hash of the block.
func (m *Miner) HeaderForWork() (types.BlockHeader, types.Target, error) {
	if !m.wallet.Unlocked() {
		return types.BlockHeader{}, types.Target{}, modules.ErrLockedWallet
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.checkAddress()
	if err != nil {
		return types.BlockHeader{}, types.Target{}, err
	}

	if time.Since(m.sourceBlockAge).Seconds() > secondsBetweenBlocks {
		m.prepareNewBlock()
	}

	// The header that will be returned for nonce grinding.
	// The header is constructed from a block and some arbitrary data. The
	// arbitrary data allows for multiple unique blocks to be generated from
	// a single block in memory. A block pointer is used in order to avoid
	// storing multiple copies of the same block in memory
	var header types.BlockHeader
	var arbData []byte
	var block *types.Block

	if m.memProgress%(headerForWorkMemory/blockForWorkMemory) == 0 {
		// Grab a new block. Allocate space for the pointer to store it as well
		block = new(types.Block)
		*block, _ = m.blockForWork()
		header = block.Header()
		arbData = block.Transactions[0].ArbitraryData[0]

		m.sourceBlockAge = time.Now()
	} else {
		// Set block to previous block, but create new arbData
		block = m.blockMem[m.headerMem[m.memProgress-1]]
		arbData, _ = crypto.RandBytes(types.SpecifierLen)
		block.Transactions[0].ArbitraryData[0] = arbData
		header = block.Header()
	}

	// Save a mapping from the header to its block as well as from the
	// header to its arbitrary data, replacing the block that was
	// stored 'headerForWorkMemory' requests ago.
	delete(m.blockMem, m.headerMem[m.memProgress])
	delete(m.arbDataMem, m.headerMem[m.memProgress])
	m.blockMem[header] = block
	m.arbDataMem[header] = arbData
	m.headerMem[m.memProgress] = header
	m.memProgress++
	if m.memProgress == headerForWorkMemory {
		m.memProgress = 0
	}

	// Return the header and target.
	return header, m.target, nil
}

// SubmitBlock takes a solved block and submits it to the blockchain.
// SubmitBlock should not be called with a lock.
func (m *Miner) SubmitBlock(b types.Block) error {
	// Give the block to the consensus set.
	err := m.cs.AcceptBlock(b)
	if err != nil {
		m.tpool.PurgeTransactionPool()
		m.log.Println("ERROR: an invalid block was submitted:", err)
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Grab a new address for the miner. Call may fail if the wallet is locked
	// or if the wallet addresses have been exhausted.
	m.blocksFound = append(m.blocksFound, b.ID())
	var uc types.UnlockConditions
	uc, err = m.wallet.NextAddress()
	if err == nil { // Special case: only update the address if there was no error.
		m.address = uc.UnlockHash()
	}
	return err
}

// SubmitHeader accepts a block header.
func (m *Miner) SubmitHeader(bh types.BlockHeader) error {
	// Fetch the block from the blockMem.
	var zeroNonce [8]byte
	lookupBH := bh
	lookupBH.Nonce = zeroNonce
	m.mu.Lock()
	b, bExists := m.blockMem[lookupBH]
	arbData, arbExists := m.arbDataMem[lookupBH]
	m.mu.Unlock()
	if !bExists || !arbExists {
		err := errors.New("block header returned late - block was cleared from memory")
		m.log.Println("ERROR:", err)
		return err
	}

	b.Transactions[0].ArbitraryData[0] = arbData
	b.Nonce = bh.Nonce
	return m.SubmitBlock(*b)
}
