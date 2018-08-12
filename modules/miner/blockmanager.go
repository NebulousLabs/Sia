package miner

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
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

	// Update the timestamp.
	if b.Timestamp < types.CurrentTimestamp() {
		b.Timestamp = types.CurrentTimestamp()
	}

	// Update the address + payouts.
	err := m.checkAddress()
	if err != nil {
		m.log.Println(err)
	}
	b.MinerPayouts = []types.SiacoinOutput{{
		Value:      b.CalculateSubsidy(m.persist.Height + 1),
		UnlockHash: m.persist.Address,
	}}

	// Add an arb-data txn to the block to create a unique merkle root.
	randBytes := fastrand.Bytes(types.SpecifierLen)
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
// min(10 minutes, 10e3 requests).
func (m *Miner) HeaderForWork() (types.BlockHeader, types.Target, error) {
	if err := m.tg.Add(); err != nil {
		return types.BlockHeader{}, types.Target{}, err
	}
	defer m.tg.Done()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a blank header with an error if the wallet is locked.
	unlocked, err := m.wallet.Unlocked()
	if err != nil {
		return types.BlockHeader{}, types.Target{}, err
	}
	if !unlocked {
		return types.BlockHeader{}, types.Target{}, modules.ErrLockedWallet
	}

	// Check that the wallet has been initialized, and that the miner has
	// successfully fetched an address.
	err = m.checkAddress()
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
	fastrand.Read(arbData[:])
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

// managedSubmitBlock takes a solved block and submits it to the blockchain.
func (m *Miner) managedSubmitBlock(b types.Block) error {
	// Give the block to the consensus set.
	err := m.cs.AcceptBlock(b)
	// Add the miner to the blocks list if the only problem is that it's stale.
	if err == modules.ErrNonExtendingBlock {
		m.mu.Lock()
		m.persist.BlocksFound = append(m.persist.BlocksFound, b.ID())
		m.mu.Unlock()
		m.log.Println("Mined a stale block - block appears valid but does not extend the blockchain")
		return err
	}
	if err == modules.ErrBlockUnsolved {
		m.log.Println("Mined an unsolved block - header submission appears to be incorrect")
		return err
	}
	if err != nil {
		m.tpool.PurgeTransactionPool()
		m.log.Critical("ERROR: an invalid block was submitted:", err)
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Grab a new address for the miner. Call may fail if the wallet is locked
	// or if the wallet addresses have been exhausted.
	m.persist.BlocksFound = append(m.persist.BlocksFound, b.ID())
	var uc types.UnlockConditions
	uc, err = m.wallet.NextAddress()
	if err != nil {
		return err
	}
	m.persist.Address = uc.UnlockHash()
	return m.saveSync()
}

// SubmitHeader accepts a block header.
func (m *Miner) SubmitHeader(bh types.BlockHeader) error {
	if err := m.tg.Add(); err != nil {
		return err
	}
	defer m.tg.Done()

	// Because a call to managedSubmitBlock is required at the end of this
	// function, the first part needs to be wrapped in an anonymous function
	// for lock safety.
	var b types.Block
	err := func() error {
		m.mu.Lock()
		defer m.mu.Unlock()

		// Lookup the block that corresponds to the provided header.
		nonce := bh.Nonce
		bh.Nonce = [8]byte{}
		bPointer, bExists := m.blockMem[bh]
		arbData, arbExists := m.arbDataMem[bh]
		if !bExists || !arbExists {
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
		bh.Nonce = nonce
		if types.BlockID(crypto.HashObject(bh)) != b.ID() {
			m.log.Critical("block reconstruction failed")
		}
		return nil
	}()
	if err != nil {
		m.log.Println("ERROR during call to SubmitHeader, pre SubmitBlock:", err)
		return err
	}
	err = m.managedSubmitBlock(b)
	if err != nil {
		m.log.Println("ERROR returned by managedSubmitBlock:", err)
		return err
	}
	return nil
}
