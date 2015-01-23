package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// The miner is used by the Core to facilitate the mining of blocks.
type Miner interface {
	// Threads returns the number of threads being used by the miner.
	Threads() int

	// Establishes the number of threads that the miner should be mining on.
	SetThreads(int) error

	// TODO: Depricate
	SetBlockChan(chan consensus.Block)

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
