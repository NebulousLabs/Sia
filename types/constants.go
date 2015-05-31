package types

// constants.go contains the Sia constants. Depending on which build tags are
// used, the constants will be initialized to different values.
//
// CONTRIBUTE: We don't have way to check that the non-test constants are all
// sane, plus we have no coverage for them.

import (
	"math/big"
	"time"

	"github.com/NebulousLabs/Sia/build"
)

var (
	BlockSizeLimit         uint64
	BlockFrequency         BlockHeight
	TargetWindow           BlockHeight
	MedianTimestampWindow  int
	ExtremeFutureThreshold Timestamp
	FutureThreshold        Timestamp
	SiafundCount           uint64
	SiafundPortion         float64

	InitialCoinbase uint64
	MinimumCoinbase uint64

	RenterZeroConfDelay time.Duration // TODO: This shouldn't exist here.

	MaturityDelay BlockHeight

	GenesisTimestamp         Timestamp
	GenesisSiafundUnlockHash = ZeroUnlockHash
	GenesisClaimUnlockHash   = ZeroUnlockHash

	RootTarget Target
	RootDepth  Target

	MaxAdjustmentUp   *big.Rat
	MaxAdjustmentDown *big.Rat

	CoinbaseAugment *big.Int
)

// init checks which build constant is in place and initializes the variables
// accordingly.
func init() {
	// Constants that are consistent regardless of the build settings.
	BlockSizeLimit = 1e6       // 1 MB
	MedianTimestampWindow = 11 // 11 Blocks.
	RootDepth = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

	SiafundCount = 10e3 // 10,000 total siafunds.
	SiafundPortion = 0.039
	InitialCoinbase = 300e3
	CoinbaseAugment = new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)
	GenesisSiafundUnlockHash = ZeroUnlockHash
	GenesisClaimUnlockHash = ZeroUnlockHash

	// Constants that depend on build settings.
	if build.Release == "dev" {
		// 'dev' settings are for small developer testnets, usually on the same
		// computer. Settings are slow enough that a small team of developers
		// can coordinate their actions over a the developer testnets, but fast
		// enough that there isn't much time wasted on waiting for things to
		// happen.
		BlockFrequency = 6 // 6 seconds: slow enough for developers to see ~each block, fast enough that blocks don't waste time.
		TargetWindow = 40  // Difficulty is adjusted based on prior 40 blocks.
		MaturityDelay = 10
		FutureThreshold = 2 * 60                 // 2 minutes.
		ExtremeFutureThreshold = 4 * 60          // 4 minutes.
		GenesisTimestamp = Timestamp(1424139000) // Approx. Feb 16th, 2015

		MaxAdjustmentUp = big.NewRat(102, 100)
		MaxAdjustmentDown = big.NewRat(98, 100)
		RootTarget = Target{0, 0, 4} // Standard developer CPUs should be able to mine blocks.
		MinimumCoinbase = 30e3

		RenterZeroConfDelay = 15 * time.Second
	} else if build.Release == "testing" {
		// 'testing' settings are for automatic testing, and create much faster
		// environments than a humand can interact with.
		BlockFrequency = 1  // As fast as possible
		TargetWindow = 10e3 // Large to prevent the difficulty from increasing during testing.
		MaturityDelay = 3
		FutureThreshold = 3        // 3 seconds
		ExtremeFutureThreshold = 6 // 6 seconds
		GenesisTimestamp = CurrentTimestamp()

		// A really restrictive difficulty clamp prevents the difficulty from
		// climbing during testing, as the resolution on the difficulty
		// adjustment is only 1 second and testing mining should be happening
		// substantially faster than that.
		MaxAdjustmentUp = big.NewRat(10001, 10000)
		MaxAdjustmentDown = big.NewRat(9999, 10000)
		RootTarget = Target{64}  // Takes an expected 4 hashes; very fast for testing but still probes 'bad hash' code.
		MinimumCoinbase = 299990 // Minimum coinbase is hit after 10 blocks to make testing minimum-coinbase code easier.

		RenterZeroConfDelay = 2 * time.Second
	} else if build.Release == "standard" {
		// 'standard' settings are for the full network. They are slow enough
		// that the network is secure in a real-world byzantine environment.
		BlockFrequency = 600                     // 1 block per 10 minutes.
		TargetWindow = 1e3                       // Number of blocks to use when calculating the target.
		MaturityDelay = 50                       // 8 hours - 50 blocks.
		FutureThreshold = 3 * 60 * 60            // 3 hours.
		ExtremeFutureThreshold = 5 * 60 * 60     // 5 hours.
		GenesisTimestamp = Timestamp(1431000000) // 12:00pm UTC May 7th 2015

		// A difficulty clamp to make long range attacks difficult. Quadrupling the
		// difficulty will take 3000x work of finding a single block of the
		// original difficulty. This can be compared to Bitcoin's clamp, in which
		// quadrupling the difficulty takes 2000x work of finding a single block of
		// the original difficulty.
		MaxAdjustmentUp = big.NewRat(1001, 1000)
		MaxAdjustmentDown = big.NewRat(999, 1000)
		RootTarget = Target{0, 0, 0, 64}
		MinimumCoinbase = 30e3

		RenterZeroConfDelay = 60 * time.Second
	}
}
