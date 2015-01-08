package components

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

	// Threads returns the number of threads being used by the miner.
	Threads() int

	// SubsidyAddress returns the address that is currently being used by the
	// miner while looking for a block.
	SubsidyAddress() consensus.CoinAddress

	// Update allows the state to change the block channel, the number of
	// threads, and the block mining information.
	//
	// If MinerUpdate.Threads == 0, the number of threads is kept the same.
	// There should be a cleaner way of doing this.
	UpdateMiner(MinerUpdate) error

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
