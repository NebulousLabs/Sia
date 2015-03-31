package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// MinerStatus is the information that gets returned to the front end. Each
// item is returned in the format that it's meant to be displayed.
type MinerInfo struct {
	State          string
	Mining         bool
	Threads        int
	RunningThreads int
	Address        consensus.UnlockHash
}

// The Miner interface provides access to mining features.
type Miner interface {
	// FindBlock will have the miner make 1 attempt to find a solved block that
	// builds on the current consensus set. It will give up after a few
	// seconds, returning a block, a bool indicating whether the block is
	// sovled, and an error.
	FindBlock() (consensus.Block, bool, error)

	// MinerInfo returns a MinerInfo struct, containing information about the
	// miner.
	MinerInfo() MinerInfo

	// MinerSubscribe is a channel to inform subscribers of when the miner has
	// updated. This is primarily used for synchronization during testing.
	MinerSubscribe() <-chan struct{}

	// SetThreads sets the number of threads in the miner.
	SetThreads(int) error

	// SolveBlock will have the miner make 1 attempt to solve the input block.
	// It will give up after a few seconds, returning the block, a bool
	// indicating whether it has been solved, and an error.
	SolveBlock(consensus.Block, consensus.Target) (consensus.Block, bool)

	// StartMining turns on the miner, which will endlessly work for new
	// blocks.
	StartMining() error

	// StopMining turns off the miner, but keeps the same number of threads.
	StopMining() error
}
