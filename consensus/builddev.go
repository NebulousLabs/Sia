// +build dev

package consensus

import (
	"math/big"
)

// Though these are variables, they should never be changed during runtime.
// They get altered during testing.
//
// TODO: on startup, there should be a check that panics if one of the
// constants has a funky or unusable value. For example, MedianTimestampWindow
// should really be an odd number.
const (
	DEBUG = true

	BlockSizeLimit        = 1024 * 1024            // Blocks cannot be more than 1MB.
	BlockFrequency        = Timestamp(10)          // In seconds.
	TargetWindow          = BlockHeight(80)        // Number of blocks to use when calculating the target.
	MedianTimestampWindow = 11                     // Number of blocks that get considered when determining if a timestamp is valid.
	FutureThreshold       = Timestamp(3 * 60 * 60) // Seconds into the future block timestamps are valid.

	InitialCoinbase = Currency(300000)
	MinimumCoinbase = Currency(30000)

	GenesisTimestamp = Timestamp(1417070299) // Approx. 1:47pm EST Nov. 13th, 2014
)

var (
	RootTarget = Target{0, 0, 8}
	RootDepth  = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

	MaxAdjustmentUp   = big.NewRat(103, 100)
	MaxAdjustmentDown = big.NewRat(97, 100)

	GenesisAddress = CoinAddress{}
)
