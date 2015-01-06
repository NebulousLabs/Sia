package sia

import (
	"github.com/NebulousLabs/Sia/sia/components"
)

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

// UpdateMiner needs to be called with the state read-locked. UpdateMiner takes
// a miner as input and calls `miner.Update()` with all of the recent values
// from the state.
func (c *Core) UpdateMiner(threads int) (err error) {
	// Get a new address if the recent block belongs to us, otherwise use the
	// current address.
	recentBlock := c.state.CurrentBlock()
	address := c.miner.SubsidyAddress()
	if address == recentBlock.MinerAddress {
		address, err = c.wallet.CoinAddress()
		if err != nil {
			return
		}
	}

	// Create the update struct for the miner.
	update := components.MinerUpdate{
		Parent:            recentBlock.ID(),
		Transactions:      c.state.TransactionPoolDump(),
		Target:            c.state.CurrentTarget(),
		Address:           address,
		EarliestTimestamp: c.state.EarliestLegalTimestamp(),

		BlockChan: c.BlockChan(),
		Threads:   threads,
	}

	// Call update on the miner.
	c.miner.Update(update)
	return
}
