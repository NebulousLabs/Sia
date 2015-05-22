package miner

import (
	"math"
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

	floatMaxTarget, _ := big.NewRat(0, 1).SetInt(types.RootDepth.Int()).Float64()
	floatCurTarget, _ := big.NewRat(0, 1).SetInt(m.target.Int()).Float64()
	hashesRequired := math.Exp2(math.Log2(floatMaxTarget) - math.Log2(floatCurTarget))
	hashesPerMonth := big.NewInt(0).Mul(big.NewInt(60*60*24*30), big.NewInt(m.hashRate))
	floatHPM, _ := big.NewRat(0, 1).SetInt(hashesPerMonth).Float64()
	blocksPerMonth := floatHPM / hashesRequired
	info := modules.MinerInfo{
		Threads:        m.threads,
		RunningThreads: m.runningThreads,
		Address:        m.address,
		HashRate:       m.hashRate,
		BlocksPerMonth: blocksPerMonth,
	}
	if info.RunningThreads != 0 {
		info.Mining = true
	}

	// Set the running info based on desiredThreads vs. runningThreads.
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
