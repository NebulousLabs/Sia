package modules

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

// MinerStatus is the information that gets returned to the front end. Each
// item is returned in the format that it's meant to be displayed.
type MinerInfo struct {
	State          string
	Mining         bool
	Threads        int
	RunningThreads int
	HashRate       int64
	BlocksPerWeek  float64
	BlocksMined    int
	OrphansMined   int
	Address        types.UnlockHash
}

// The Miner interface provides access to mining features.
type Miner interface {
	// BlockForWork returns a block that is ready for nonce grinding. All
	// blocks returned by BlockForWork have a unique merkle root, meaning that
	// each can safely start from nonce 0.
	BlockForWork() (types.Block, crypto.Hash, types.Target)

	// FindBlock will have the miner make 1 attempt to find a solved block that
	// builds on the current consensus set. It will give up after a few
	// seconds, returning a block, a bool indicating whether the block is
	// sovled, and an error. FindBlock will submit the block to the consensus
	// set.
	FindBlock() (types.Block, bool, error)

	// MinerInfo returns a MinerInfo struct, containing information about the
	// miner.
	MinerInfo() MinerInfo

	// MinerNotify is a channel to inform subscribers of when the miner has
	// updated. This is primarily used for synchronization during testing.
	MinerNotify() <-chan struct{}

	// SetThreads sets the number of threads in the miner.
	SetThreads(int) error

	// SolveBlock will have the miner make 1 attempt to solve the input block.
	// It will give up after a few seconds, returning the block, a bool
	// indicating whether it has been solved, and an error. SolveBlock is very
	// slow, and will not submit the block the the consensus set.
	SolveBlock(types.Block, types.Target) (types.Block, bool)

	// StartMining turns on the miner, which will endlessly work for new
	// blocks.
	StartMining() error

	// StopMining turns off the miner, but keeps the same number of threads.
	StopMining() error

	// SubmitBlock takes a block that has been worked on and has a valid
	// target. Typically used with external miners.
	SubmitBlock(types.Block) error
}
