// +build test

package consensus

import (
	"math/big"
)

const (
	DEBUG = true

	BlockSizeLimit        = 1024 * 1024 // Blocks cannot be more than 1MB.
	BlockFrequency        = 1           // In seconds.
	TargetWindow          = 1000        // Number of blocks to use when calculating the target.
	MedianTimestampWindow = 11          // Number of blocks that get considered when determining if a timestamp is valid - should be an odd number.
	FutureThreshold       = 3 * 60 * 60 // Seconds into the future block timestamps are valid.
	SiafundCount          = 1e4         // The total (static) number of siafunds.

	InitialCoinbase = 3e5
	MinimumCoinbase = 3e4

	GenesisTimestamp = Timestamp(1417070299) // Approx. 1:47pm EST Nov. 13th, 2014
)

var (
	RootTarget = Target{64}
	RootDepth  = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

	MaxAdjustmentUp   = big.NewRat(1001, 1000)
	MaxAdjustmentDown = big.NewRat(999, 1000)
)
