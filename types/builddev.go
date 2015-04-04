// +build dev

package types

import (
	"math/big"
)

const (
	DEBUG = true

	BlockSizeLimit        = 1e6         // Blocks cannot be more than 1MB.
	BlockFrequency        = 6           // In seconds.
	TargetWindow          = 40          // Number of blocks to use when calculating the target.
	MedianTimestampWindow = 11          // Number of blocks that get considered when determining if a timestamp is valid. Should be an odd number.
	FutureThreshold       = 3 * 60 * 60 // Seconds into the future block timestamps are valid.
	SiafundCount          = 10e3        // The total (static) number of siafunds.
	SiafundPortion        = 0.039       // Percent of all contract payouts that go to the siafund pool.
	MaturityDelay         = 10          // The number of blocks that need to be waited before certain types of outputs come to maturity.
	InitialCoinbase       = 300e3
	MinimumCoinbase       = 30e3

	RenterZeroConfDelay = 60e9 // 1 minute
)

var (
	RootTarget = Target{0, 0, 64}
	RootDepth  = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

	MaxAdjustmentUp   = big.NewRat(102, 100)
	MaxAdjustmentDown = big.NewRat(98, 100)

	CoinbaseAugment = new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)

	GenesisTimestamp         = Timestamp(1424139000) // Approx. Feb. 16th, 2015.
	GenesisSiafundUnlockHash = ZeroUnlockHash
	GenesisClaimUnlockHash   = ZeroUnlockHash
)
