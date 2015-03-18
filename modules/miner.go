package modules

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// The Miner interface provides access to mining features.
type Miner interface {
	// FindBlock will have the miner make 1 attempt to find a solved block that
	// builds on the current consensus set. It will give up after a few
	// seconds, returning a block, a bool indicating whether the block is
	// sovled, and an error.
	FindBlock() (consensus.Block, bool, error)

	// SolveBlock will have the miner make 1 attempt to solve the input block.
	// It will give up after a few seconds, returning the block, a bool
	// indicating whether it has been solved, and an error.
	SolveBlock(consensus.Block, consensus.Target) (consensus.Block, bool)
}
