package components

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// MinerStatus is the information that gets returned to the front end. Each
// item is returned in the format that it's meant to be displayed.
type MinerInfo struct {
	State          string
	Threads        int
	RunningThreads int
	Address        consensus.CoinAddress
}

// The miner is used by the Core to facilitate the mining of blocks.
type Miner interface {
	// Info returns an arbitrary byte slice presumably with information about
	// the status of the miner. Info is not relevant to the sia package, but
	// instead to the front end.
	Info() (MinerInfo, error)

	// Threads returns the number of threads being used by the miner.
	Threads() int

	// Establishes the number of threads that the miner should be mining on.
	SetThreads(int) error

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
