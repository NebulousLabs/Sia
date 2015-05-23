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
	BlocksPerMonth float64
	Address        types.UnlockHash
}

// MinerWork is all the information a miner needs to do work
type MinerWork struct {
	Block      types.Block
	ParentID   types.BlockID
	Nonce      uint64
	MerkleRoot crypto.Hash
	Target     types.Target
}

// The Miner interface provides access to mining features.
type Miner interface {
	// FindBlock will have the miner make 1 attempt to find a solved block that
	// builds on the current consensus set. It will give up after a few
	// seconds, returning a block, a bool indicating whether the block is
	// sovled, and an error.
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
	// indicating whether it has been solved, and an error.
	SolveBlock(types.Block, types.Target) (types.Block, bool)

	// GetWork() returns a MinerWork struct which can be converted to JSON to be
	// parsed by external miners
	GetWork() MinerWork

	// StartMining turns on the miner, which will endlessly work for new
	// blocks.
	StartMining() error

	// StopMining turns off the miner, but keeps the same number of threads.
	StopMining() error
}
