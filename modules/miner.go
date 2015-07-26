package modules

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

const (
	MinerDir = "miner"
)

// A BlockManager contains functions that can interface with external miners,
// providing and receiving blocks that have experienced nonce grinding.
type BlockManager interface {
	// BlockForWork returns a block that is ready for nonce grinding. All
	// blocks returned by BlockForWork have a unique merkle root, meaning that
	// each can safely start from nonce 0.
	BlockForWork() (types.Block, crypto.Hash, types.Target)

	// HeaderForWork returns a block header that can be grinded on and
	// resubmitted to the miner. HeaderForWork() will remember the block that
	// corresponds to the header for 50 calls.
	HeaderForWork() (types.BlockHeader, types.Target)

	// SubmitBlock takes a block that has been worked on and has a valid
	// target. Typically used with external miners.
	SubmitBlock(types.Block) error

	// SubmitHeader takes a block header that has been worked on and has a
	// valid target. A superior choice to SubmitBlock.
	SubmitHeader(types.BlockHeader) error
}

// A CPUMiner provides access to a single-threaded cpu miner.
type CPUMiner interface {
	// CPUHashrate returns the hashrate of the cpu miner in hashes per second.
	CPUHashrate() int

	// Mining returns true if the cpu miner is enabled, and false otherwise.
	CPUMining() bool

	// FindBlock will have the miner make 1 attempt to find a solved block that
	// builds on the current consensus set. It will give up after a few
	// seconds, returning the block and a bool indicating whether the block is
	// sovled.
	FindBlock() (types.Block, bool)

	// StartMining turns on the miner, which will endlessly work for new
	// blocks.
	StartCPUMining()

	// StopMining turns off the miner, but keeps the same number of threads.
	StopCPUMining()

	// SolveBlock will have the miner make 1 attempt to solve the input block,
	// which amounts to trying a few thousand different nonces. SolveBlock is
	// primarily used for testing.
	SolveBlock(types.Block, types.Target) (types.Block, bool)
}

// The Miner interface provides access to mining features.
type Miner interface {
	BlockManager
	CPUMiner

	// BlocksMined returns the number of blocks and stale blocks that have been
	// mined using this miner.
	BlocksMined() (goodBlocks, staleBlocks int)
}
