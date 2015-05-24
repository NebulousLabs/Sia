package miner

import (
	"math/big"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Info() returns a MinerInfo struct which can be converted to JSON to be
// parsed by frontends for displaying information to the user.
//
// State is a string indicating what the miner is currently doing with respect
// to the number of threads it currently has vs. the number of threads it wants
// to have.
//
// Threads is the number of threads that the miner currently wants to have.
//
// RunningThreads is the number of threads that the miner currently has.
//
// Address is the current address that is receiving block payouts.
func (m *Miner) MinerInfo() modules.MinerInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Using the hashrate and target, determine the number of blocks per month
	// that could be mined.
	hashesRequired, _ := big.NewRat(0, 1).SetFrac(types.RootDepth.Int(), m.target.Int()).Float64()
	hashesPerWeek := big.NewInt(0).Mul(big.NewInt(60*60*24*7), big.NewInt(m.hashRate))
	floatHPW, _ := big.NewRat(0, 1).SetInt(hashesPerWeek).Float64()
	blocksPerWeek := floatHPW / hashesRequired

	info := modules.MinerInfo{
		Threads:        m.threads,
		RunningThreads: m.runningThreads,
		Address:        m.address,
		HashRate:       m.hashRate,
		BlocksPerWeek:  blocksPerWeek,
	}

	// Using the reference of all blocks that have been mined, determine how
	// many blocks have been mined successfully and how many have been mined as
	// orphans.
	for _, blockID := range m.blocksFound {
		if m.cs.InCurrentPath(blockID) {
			info.BlocksMined++
		} else {
			info.OrphansMined++
		}
	}

	// Set the running info based on desiredThreads vs. runningThreads.
	if info.RunningThreads != 0 {
		info.Mining = true
	}
	if m.desiredThreads == 0 && m.runningThreads == 0 {
		info.State = "Off"
	} else if m.desiredThreads == 0 && m.runningThreads > 0 {
		// If there are bugs or if the computer is slow (i.e. raspi), turning
		// off can take multiple seconds.
		info.State = "Turning Off"
	} else if m.desiredThreads == m.runningThreads {
		info.State = "On"
	}

	return info
}
