package sia

import (
	"github.com/NebulousLabs/Sia/consensus"
)

type Miner interface {
	// Info returns an arbitrary byte slice presumably with information about
	// the status of the miner. Info is not relevant to the sia package, but
	// instead to the front end.
	Info() ([]byte, error)

	// SubsidyAddress returns the address that is currently being used by the
	// miner while looking for a block.
	SubsidyAddress() consensus.CoinAddress

	// Update takes a block and a set of transactions. The miner will use the
	// given block as the parent, and will use the transactions as the set of
	// transactions in the block.
	Update(parentID consensus.BlockID, transactionSet []consensus.Transaction, nextTarget consensus.Target, subsidyAddress consensus.CoinAddress, earliestTimestamp consensus.Timestamp) error

	// StartMining takes as input a number of threads to use for mining. 0 will return
	// an error.
	StartMining() error

	// StopMining will take all of the threads down to 0. If mining is already
	// stopped, an error will be returned.
	StopMining() error

	// SolveBlock will attempt to solve a block, returning the most recent
	// attempt and indicating whether the solve was successful or not.
	SolveBlock() (block consensus.Block, solved bool, err error)
}

// StartMining calls StartMining on the miner.
func (c *Core) StartMining() error {
	return c.miner.StartMining()
}

// StopMining calls StopMining on the miner.
func (c *Core) StopMining() error {
	return c.miner.StopMining()
}

// MinerInfo calls Info on the miner.
func (c *Core) MinerInfo() ([]byte, error) {
	return c.miner.Info()
}

// updateMiner needs to be called with the state read-locked. updateMiner takes
// a miner as input and calls `miner.Update()` with all of the recent values
// from the state. Usually, but not always, the call will be
// c.updateMiner(c.miner).
func (c *Core) updateMiner(miner Miner) (err error) {
	recentBlock := c.state.CurrentBlock()
	transactionSet := c.state.TransactionPoolDump()
	target := c.state.CurrentTarget()
	earliestTimestamp := c.state.EarliestLegalTimestamp()

	// Get a new address if the recent block belongs to us, otherwise use the
	// current address.
	address := c.miner.SubsidyAddress()
	if address == recentBlock.MinerAddress {
		address, err = c.wallet.CoinAddress()
		if err != nil {
			return
		}
	}

	// Call update on the miner.
	miner.Update(recentBlock.ID(), transactionSet, target, address, earliestTimestamp)
	return
}

// ReplaceMiner terminates the existing miner and replaces it with the new
// miner. ReplaceMiner will not call `StartMining()` on the new miner.
func (c *Core) ReplaceMiner(miner Miner) {
	// Fill out the new miner with the most recent block information.
	c.state.RLock()
	c.updateMiner(miner)
	c.state.RUnlock()

	// Kill and replace the existing miner.
	c.miner.StopMining()
	c.miner = miner
}
