package miner

import (
	"crypto/rand"
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// blockForWork returns a block that is ready for nonce grinding, including
// correct miner payouts and a random transaction to prevent collisions and
// overlapping work with other blocks being mined in parallel or for different
// forks (during testing).
func (m *Miner) blockForWork() types.Block {
	b := m.unsolvedBlock

	// Update the timestmap.
	if b.Timestamp < types.CurrentTimestamp() {
		b.Timestamp = types.CurrentTimestamp()
	}

	// Update the address + payouts.
	_ = m.checkAddress() // Err is ignored - address generation failed but can't do anything about it (log maybe).
	b.MinerPayouts = []types.SiacoinOutput{types.SiacoinOutput{Value: b.CalculateSubsidy(m.height + 1), UnlockHash: m.address}}

	// Add an arb-data txn to the block to create a unique merkle root.
	randBytes, _ := crypto.RandBytes(types.SpecifierLen)
	randTxn := types.Transaction{
		ArbitraryData: [][]byte{append(modules.PrefixNonSia[:], randBytes...)},
	}
	b.Transactions = append([]types.Transaction{randTxn}, b.Transactions...)

	return b
}

// newSourceBlock creates a new source block for the block manager so that new
// headers will use the updated source block.
func (m *Miner) newSourceBlock() {
	// To guarantee garbage collection of old blocks, delete all header entries
	// that have not been reached for the current block.
	for m.memProgress%(HeaderMemory/BlockMemory) != 0 {
		delete(m.blockMem, m.headerMem[m.memProgress])
		delete(m.arbDataMem, m.headerMem[m.memProgress])
		m.memProgress++
		if m.memProgress == HeaderMemory {
			m.memProgress = 0
		}
	}

	// Update the source block.
	block := m.blockForWork()
	m.sourceBlock = &block
}

// HeaderForWork returns a header that is ready for nonce grinding. The miner
// will store the header in memory for a while, depending on the constants
// 'HeaderMemory', 'BlockMemory', and 'MaxSourceBlockAge'. On the full network,
// it is typically safe to assume that headers will be remembered for
// min(10 minutes, 1000 requests).
func (m *Miner) HeaderForWork() (types.BlockHeader, types.Target, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a blank header with an error if the wallet is locked.
	if !m.wallet.Unlocked() {
		return types.BlockHeader{}, types.Target{}, modules.ErrLockedWallet
	}

	// Check that the wallet has been initialized, and that the miner has
	// successfully fetched an address.
	err := m.checkAddress()
	if err != nil {
		return types.BlockHeader{}, types.Target{}, err
	}

	// If too much time has elapsed since the last source block, get a new one.
	// This typically only happens if the miner has just turned on after being
	// off for a while. If the current block has been used for too many
	// requests, fetch a new source block.
	if time.Since(m.sourceBlockAge) > MaxSourceBlockAge || m.memProgress%(HeaderMemory/BlockMemory) == 0 {
		m.newSourceBlock()
	}

	// Create a header from the source block.
	_, err = rand.Read(m.sourceBlock.Transactions[0].ArbitraryData[0])
	if err != nil {
		return types.BlockHeader{}, types.Target{}, err
	}
	header := m.sourceBlock.Header()

	// Save the mapping from the header to its block and from the header to its
	// arbitrary data, replacing whatever header already exists.
	delete(m.blockMem, m.headerMem[m.memProgress])
	delete(m.arbDataMem, m.headerMem[m.memProgress])
	m.blockMem[header] = m.sourceBlock
	m.arbDataMem[header] = m.sourceBlock.Transactions[0].ArbitraryData[0]
	m.headerMem[m.memProgress] = header
	m.memProgress++
	if m.memProgress == HeaderMemory {
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
	if err == nil { // Only update the address if there was no error.
		m.address = uc.UnlockHash()
	}
	return err
}

// SubmitHeader accepts a block header.
func (m *Miner) SubmitHeader(bh types.BlockHeader) error {
	// Lookup the block that corresponds to the provided header.
	var b types.Block
	nonce := bh.Nonce
	bh.Nonce = [8]byte{}
	m.mu.Lock()
	bPointer, bExists := m.blockMem[bh]
	arbData, arbExists := m.arbDataMem[bh]
	if !bExists || !arbExists {
		err := errors.New("block header returned late - block was cleared from memory")
		m.log.Println("ERROR:", err)
		m.mu.Unlock()
		return err
	}
	b = *bPointer // Must be done before releasing the lock.
	b.Transactions[0].ArbitraryData[0] = arbData
	b.Nonce = nonce
	m.mu.Unlock()

	return m.SubmitBlock(b)
}
