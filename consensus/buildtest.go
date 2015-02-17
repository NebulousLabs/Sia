// +build test

package consensus

import (
	"math/big"
)

const (
	DEBUG = true

	BlockSizeLimit        = 1e6         // Blocks cannot be more than 1MB.
	BlockFrequency        = 1           // In seconds.
	TargetWindow          = 1e3         // Number of blocks to use when calculating the target.
	MedianTimestampWindow = 11          // Number of blocks that get considered when determining if a timestamp is valid - should be an odd number.
	FutureThreshold       = 3 * 60 * 60 // Seconds into the future block timestamps are valid.
	SiafundCount          = 10e3        // The total (static) number of siafunds.
	MaturityDelay         = 3           // The number of blocks that need to be waited before certain types of outputs come to maturity.
	SiafundPortion        = 0.039       // Percent of all contract payouts that go to the siafund pool.

	InitialCoinbase = 300e3
	MinimumCoinbase = 299990 // Only takes 10 blocks to hit the max - useful for testing.
)

var (
	RootTarget = Target{64}
	RootDepth  = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

	MaxAdjustmentUp   = big.NewRat(10001, 10000)
	MaxAdjustmentDown = big.NewRat(9999, 10000)

	CoinbaseAugment = new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)

	GenesisTimestamp         = CurrentTimestamp()
	GenesisSiafundUnlockHash = ZeroUnlockHash
	GenesisClaimUnlockHash   = ZeroUnlockHash
)
