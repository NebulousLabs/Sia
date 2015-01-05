package sia

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// MinerUpdate condenses the set of inputs to the Update() function into a
// single struct.
type MinerUpdate struct {
	Parent            consensus.BlockID
	Transactions      []consensus.Transaction
	Target            consensus.Target
	Address           consensus.CoinAddress
	EarliestTimestamp consensus.Timestamp

	BlockChan chan consensus.Block
	Threads   int
}

// The miner is used by the Core to facilitate the mining of blocks.
type Miner interface {
	// Info returns an arbitrary byte slice presumably with information about
	// the status of the miner. Info is not relevant to the sia package, but
	// instead to the front end.
	Info() ([]byte, error)

	// SubsidyAddress returns the address that is currently being used by the
	// miner while looking for a block.
	SubsidyAddress() consensus.CoinAddress

	// Update allows the state to change the block channel, the number of
	// threads, and the block mining information.
	//
	// If MinerUpdate.Threads == 0, the number of threads is kept the same.
	// There should be a cleaner way of doing this.
	Update(MinerUpdate) error

	// StartMining will turn on the miner and begin consuming computational
	// cycles.
	StartMining() error

	// StopMining will turn of the miner and stop consuming computational
	// cycles.
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
	update := MinerUpdate{
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
