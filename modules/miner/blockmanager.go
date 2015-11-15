package miner

import (
	"crypto/rand"
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errLateHeader = errors.New("header is old, block could not be recovered")
)

// blockForWork returns a block that is ready for nonce grinding, including
// correct miner payouts and a random transaction to prevent collisions and
// overlapping work with other blocks being mined in parallel or for different
// forks (during testing).
func (m *Miner) blockForWork() types.Block {
	b := m.persist.UnsolvedBlock

	// Update the timestmap.
	if b.Timestamp < types.CurrentTimestamp() {
		b.Timestamp = types.CurrentTimestamp()
	}

	// Update the address + payouts.
	_ = m.checkAddress() // Err is ignored - address generation failed but can't do anything about it (log maybe).
	b.MinerPayouts = []types.SiacoinOutput{{Value: b.CalculateSubsidy(m.persist.Height + 1), UnlockHash: m.persist.Address}}

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
	m.sourceBlockTime = time.Now()
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
	if time.Since(m.sourceBlockTime) > MaxSourceBlockAge || m.memProgress%(HeaderMemory/BlockMemory) == 0 {
		m.newSourceBlock()
	}

	// Create a header from the source block - this may be a race condition,
	// but I don't think so (underlying slice may be shared with other blocks
	// accessible outside the miner).
	var arbData [crypto.EntropySize]byte
	_, err = rand.Read(arbData[:])
	if err != nil {
		return types.BlockHeader{}, types.Target{}, err
	}
	copy(m.sourceBlock.Transactions[0].ArbitraryData[0], arbData[:])
	header := m.sourceBlock.Header()

	// Save the mapping from the header to its block and from the header to its
	// arbitrary data, replacing whatever header already exists.
	delete(m.blockMem, m.headerMem[m.memProgress])
	delete(m.arbDataMem, m.headerMem[m.memProgress])
	m.blockMem[header] = m.sourceBlock
	m.arbDataMem[header] = arbData
	m.headerMem[m.memProgress] = header
	m.memProgress++
	if m.memProgress == HeaderMemory {
		m.memProgress = 0
	}

	// Return the header and target.
	return header, m.persist.Target, nil
}

// SubmitBlock takes a solved block and submits it to the blockchain.
// SubmitBlock should not be called with a lock.
func (m *Miner) SubmitBlock(b types.Block) error {
	// Give the block to the consensus set.
	err := m.cs.AcceptBlock(b)
	// Add the miner to the blocks list if the only problem is that it's stale.
	if err == modules.ErrNonExtendingBlock {
		m.mu.Lock()
		m.persist.BlocksFound = append(m.persist.BlocksFound, b.ID())
		m.mu.Unlock()
	}
	if err != nil {
		m.tpool.PurgeTransactionPool()
		m.log.Println("ERROR: an invalid block was submitted:", err)
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Grab a new address for the miner. Call may fail if the wallet is locked
	// or if the wallet addresses have been exhausted.
	m.persist.BlocksFound = append(m.persist.BlocksFound, b.ID())
	var uc types.UnlockConditions
	uc, err = m.wallet.NextAddress()
	if err == nil { // Only update the address if there was no error.
		m.persist.Address = uc.UnlockHash()
	}
	return err
}

// SubmitHeader accepts a block header.
func (m *Miner) SubmitHeader(bh types.BlockHeader) error {
	m.mu.Lock()

	// Lookup the block that corresponds to the provided header.
	var b types.Block
	nonce := bh.Nonce
	bh.Nonce = [8]byte{}
	bPointer, bExists := m.blockMem[bh]
	arbData, arbExists := m.arbDataMem[bh]
	if !bExists || !arbExists {
		m.log.Println("ERROR:", errLateHeader)
		m.mu.Unlock()
		return errLateHeader
	}

	// Block is going to be passed to external memory, but the memory pointed
	// to by the transactions slice is still being modified - needs to be
	// copied. Same with the memory being pointed to by the arb data slice.
	b = *bPointer
	txns := make([]types.Transaction, len(b.Transactions))
	copy(txns, b.Transactions)
	b.Transactions = txns
	b.Transactions[0].ArbitraryData = [][]byte{arbData[:]}
	b.Nonce = nonce

	// Sanity check - block should have same id as header.
	if build.DEBUG {
		bh.Nonce = nonce
		if types.BlockID(crypto.HashObject(bh)) != b.ID() {
			panic("block reconstruction failed")
		}
	}

	m.mu.Unlock()
	return m.SubmitBlock(b)
}
