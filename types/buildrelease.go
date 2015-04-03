// +build !test,!dev

package types

import (
	"math/big"
)

const (
	DEBUG = true // This is a temporary setting, will stay during beta.

	BlockSizeLimit        = 1e6         // Blocks cannot be more than 1MB.
	BlockFrequency        = 600         // In seconds.
	TargetWindow          = 1e3         // Number of blocks to use when calculating the target.
	MedianTimestampWindow = 11          // Number of blocks that get considered when determining if a timestamp is valid - should be an odd number.
	FutureThreshold       = 3 * 60 * 60 // Seconds into the future block timestamps are valid.
	SiafundCount          = 10e3        // The total (static) number of siafunds.
	MaturityDelay         = 50          // The number of blocks that need to be waited before certain types of outputs come to maturity.
	SiafundPortion        = 0.039       // Percent of all contract payouts that go to the siafund pool.

	InitialCoinbase = 300e3
	MinimumCoinbase = 30e3

	RenterZeroConfDelay = 60e9 // 1 minute
)

var (
	RootTarget = Target{0, 0, 0, 64}
	RootDepth  = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

	MaxAdjustmentUp   = big.NewRat(1001, 1000)
	MaxAdjustmentDown = big.NewRat(999, 1000)

	CoinbaseAugment = new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)

	GenesisTimestamp         = Timestamp(1426537000) // Approx. 4:16pm EST Mar. 16th, 2015
	GenesisSiafundUnlockHash = ZeroUnlockHash
	GenesisClaimUnlockHash   = ZeroUnlockHash
)
