package miner

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/NebulousLabs/Sia/types"
)

// BlockForWork returns a block that is ready for nonce grinding, along with
// the root hash of the block.
func (m *Miner) HeaderForWork() (types.BlockHeader, types.Target) {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	// Save a mapping between the block and its header, replacing the block
	// that was stored 'headerForWorkMemory' requests ago.
	delete(m.blockMem, m.headerMem[m.memProgress])
	m.blockMem[b.Header()] = b
	m.headerMem[m.memProgress] = b.Header()
	m.memProgress++
	if m.memProgress == headerForWorkMemory {
		m.memProgress = 0
	}

	// Return the header and target.
	return b.Header(), m.target
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
		fmt.Println("block returned too late - too many HeaderForWork().")
		return errors.New("block returned too late - has already been cleared from memory")
	}
	b.Nonce = bh.Nonce

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
