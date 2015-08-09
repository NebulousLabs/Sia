package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	MiningPoolDir = "miningpool"
)

// The MiningPool interface provides functions that allow external miners
// mine for the pool
type MiningPool interface {
	// Settings returns the host's settings
	Settings() MiningPoolSettings

	// Should there be some API call to communicate which side of a fork the pool is on?
}

type MiningPoolSettings struct {
	// The wallet address the miner should pay to
	Address types.UnlockHash

	// The target multiple for this specfic pool
	TargetMultiple uint32

	// Miners will mine blocks which pay MinerPercentCut of the subsidy to
	// themselves and the rest to the pool. The pool takes its cut then and
	// keeps PoolPercentCut then uses the rest to pay miners based on work. A
	// partial block is therefore worth:
	// subsidy * ((1 - MinerPercentCut/100) * PoolPercentCut/100) / TargetMultiple
	PoolPercentCut  uint8
	MinerPercentCut uint8
}
