// +build !test,!dev

package consensus

import (
	"math/big"
)

// Though these are variables, they should never be changed during runtime.
// They get altered during testing.
const (
	DEBUG = true // This is a temporary setting, will stay during beta.

	BlockSizeLimit        = 1024 * 1024            // Blocks cannot be more than 1MB.
	BlockFrequency        = Timestamp(600)         // In seconds.
	TargetWindow          = BlockHeight(2000)      // Number of blocks to use when calculating the target.
	MedianTimestampWindow = 11                     // Number of blocks that get considered when determining if a timestamp is valid - should be an odd number.
	FutureThreshold       = Timestamp(3 * 60 * 60) // Seconds into the future block timestamps are valid.

	InitialCoinbase = Currency(300000)
	MinimumCoinbase = Currency(30000)

	GenesisTimestamp = Timestamp(1417070299) // Approx. 1:47pm EST Nov. 13th, 2014
)

var (
	RootTarget = Target{0, 0, 0, 64}
	RootDepth  = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

	MaxAdjustmentUp   = big.NewRat(1001, 1000)
	MaxAdjustmentDown = big.NewRat(999, 1000)

	GenesisAddress = CoinAddress{} // TODO: get a hardcoded premine address
)
