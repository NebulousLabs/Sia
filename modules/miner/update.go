package miner

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// threadedConsensusRescan resets all of the miner consensus variables and
// scans the blockchain starting from the genesis block. This function is
// typically only needed if an error during startup indicates that the miner
// has desynchronized from the consensus set. An error will be sent down the
// input error channel only after the function has completed.
func (m *Miner) threadedConsensusRescan(c chan error) {
	// Unsubscribe the miner from the consensus set. Though typically
	// miner.consensusRescan will only be called if the miner is not yet
	// subscribed successfully to the consensus set, the function is allowed to
	// be used in other ways.
	m.cs.Unsubscribe(m)

	// Reset all of the variables that have relevance to the consensus set. The
	// operations are wrapped by an anonymous function so that the locking can
	// be handled using a defer statement.
	err := func() error {
		m.mu.Lock()
		defer m.mu.Unlock()

		m.persist.RecentChange = modules.ConsensusChangeID{}
		m.persist.Height = 0
		m.persist.Target = types.Target{}
		err := m.save()
		if err != nil {
			return err
		}
		return nil
	}()
	if err != nil {
		c <- err
		return
	}

	// ConsensusSetPerscribe is a blocking call that will not return until
	// rescanning is complete.
	c <- m.cs.ConsensusSetPersistentSubscribe(m, modules.ConsensusChangeID{})
	return
}

// ProcessConsensusDigest will update the miner's most recent block.
func (m *Miner) ProcessConsensusChange(cc modules.ConsensusChange) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Adjust the height of the miner. The miner height is initialized to zero,
	// but the genesis block is actually height zero. For the genesis block
	// only, the height will be left at zero.
	//
	// Checking the height here eliminates the need to initialize the miner to
	// an underflowed types.BlockHeight, which was deemed the worse of the two
	// evils.
	if m.persist.Height != 0 || cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID() != m.cs.GenesisBlock().ID() {
		m.persist.Height -= types.BlockHeight(len(cc.RevertedBlocks))
		m.persist.Height += types.BlockHeight(len(cc.AppliedBlocks))
	}

	// Update the unsolved block.
	var exists1, exists2 bool
	m.persist.UnsolvedBlock.ParentID = cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID()
	m.persist.Target, exists1 = m.cs.ChildTarget(m.persist.UnsolvedBlock.ParentID)
	m.persist.UnsolvedBlock.Timestamp, exists2 = m.cs.MinimumValidChildTimestamp(m.persist.UnsolvedBlock.ParentID)
	if build.DEBUG && !exists1 {
		panic("could not get child target")
	}
	if build.DEBUG && !exists2 {
		panic("could not get child earliest timestamp")
	}

	// There is a new parent block, the source block should be updated to keep
	// the stale rate as low as possible.
	m.newSourceBlock()
	m.persist.RecentChange = cc.ID

	// Save the new consensus information.
	err := m.save()
	if err != nil {
		m.log.Println("ERROR: could not save during ProcessConsensusChange:", err)
	}
}

// ReceiveUpdatedUnconfirmedTransactions will replace the current unconfirmed
// set of transactions with the input transactions.
func (m *Miner) ReceiveUpdatedUnconfirmedTransactions(unconfirmedTransactions []types.Transaction, _ modules.ConsensusChange) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Edge case - if there are no transactions, set the block's transactions
	// to nil and return.
	if len(unconfirmedTransactions) == 0 {
		m.persist.UnsolvedBlock.Transactions = nil
		return
	}

	// Add transactions to the block until the block size limit is reached.
	// Transactions are assumed to be in a sensible order.
	var i int
	remainingSize := int(types.BlockSizeLimit - 5e3)
	for i = range unconfirmedTransactions {
		remainingSize -= len(encoding.Marshal(unconfirmedTransactions[i]))
		if remainingSize < 0 {
			break
		}
	}
	m.persist.UnsolvedBlock.Transactions = unconfirmedTransactions[:i+1]
}
