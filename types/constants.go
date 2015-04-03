package types

import (
	"math/big"
	"time"

	"github.com/NebulousLabs/Sia/build"
)

// constants.go contains the Sia constants. Depending on which build tags are
// used, the constants will be initialized to different values.  and therefore
// cannot be declared as constants.

var (
	BlockSizeLimit        uint64
	BlockFrequency        BlockHeight
	TargetWindow          BlockHeight
	MedianTimestampWindow int
	FutureThreshold       Timestamp
	SiafundCount          uint64
	SiafundPortion        float64

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
	if build.Release == "dev" {
		BlockSizeLimit = 1e6          // 1 MB
		BlockFrequency = 6            // 6 seconds: slow enough for developers to see ~each block, fast enough that blocks don't waste time.
		TargetWindow = 40             // Difficulty is adjusted based on prior 40 blocks.
		MedianTimestampWindow = 11    // 11 Blocks.
		FutureThreshold = 3 * 60 * 60 // 3 hours.
		SiafundCount = 10e3           // 10,000 total siafunds.
		SiafundPortion = 0.039

		InitialCoinbase = 300e3
		MinimumCoinbase = 30e3

		RenterZeroConfDelay = 60 * time.Second

		// Some types of siacoin outputs cannot be collected for 10 blocks, this is
		// high enough for developers to easily see that it works (1 minute), but
		// low enough that it doesn't waste time.
		MaturityDelay = 10

		RootTarget = Target{0, 0, 64} // Standard developer CPUs should be able to mine blocks.
		RootDepth = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

		// The difficulty will adjust quickly.
		MaxAdjustmentUp = big.NewRat(102, 100)
		MaxAdjustmentDown = big.NewRat(98, 100)

		CoinbaseAugment = new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)

		GenesisTimestamp = Timestamp(1424139000) // Approx. Feb 16th, 2015
		GenesisSiafundUnlockHash = ZeroUnlockHash
		GenesisClaimUnlockHash = ZeroUnlockHash
	} else if build.Release == "testing" {
		BlockSizeLimit = 1e6
		BlockFrequency = 1  // As fast as possible
		TargetWindow = 10e3 // Large to prevent the difficulty from increasing during testing.
		MedianTimestampWindow = 11
		FutureThreshold = 3 * 60 * 60
		SiafundCount = 10e3
		SiafundPortion = 0.039

		InitialCoinbase = 300e3
		MinimumCoinbase = 299990 // Minimum coinbase is hit after 10 blocks to make testing minimum-coinbase code easier.

		RenterZeroConfDelay = 2 * time.Second

		MaturityDelay = 3

		RootTarget = Target{64} // Takes an expected 4 hashes; very fast for testing but still probes 'bad hash' code.
		RootDepth = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

		MaxAdjustmentUp = big.NewRat(10001, 10000) // Small to prevent the difficulty from increasing during testing.
		MaxAdjustmentDown = big.NewRat(9999, 10000)

		CoinbaseAugment = new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)

		GenesisTimestamp = CurrentTimestamp()
		GenesisSiafundUnlockHash = ZeroUnlockHash
		GenesisClaimUnlockHash = ZeroUnlockHash
	} else if build.Release == "standard" {
		BlockSizeLimit = 1e6          // 1MB
		BlockFrequency = 600          // 1 block per 10 minutes.
		TargetWindow = 1e3            // Number of blocks to use when calculating the target.
		MedianTimestampWindow = 11    // 11 blocks - number of blocks used when calculating the minimum allowed timestamp.
		FutureThreshold = 3 * 60 * 60 // Seconds into the future block timestamps are valid.
		SiafundCount = 10e3           // The total (static) number of siafunds.
		SiafundPortion = 0.039        // Percent of all contract payouts that go to the siafund pool.

		InitialCoinbase = 300e3
		MinimumCoinbase = 30e3

		RenterZeroConfDelay = 60 * time.Second

		// Some types of siacoin outputs cannot be collected for 50 blocks, or
		// about 8 hours. This makes it difficult to spend money that may not exist
		// in the future. Reorganizations 50 deep would be required, which takes a
		// lot of resources.
		MaturityDelay = 50

		RootTarget = Target{0, 0, 0, 64}
		RootDepth = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

		// A difficulty clamp to make long range attacks difficult. Quadrupling the
		// difficulty will take 3000x work of finding a single block of the
		// original difficulty. This can be compared to Bitcoin's clamp, in which
		// quadrupling the difficulty takes 2000x work of finding a single block of
		// the original difficulty.
		MaxAdjustmentUp = big.NewRat(1001, 1000)
		MaxAdjustmentDown = big.NewRat(999, 1000)

		CoinbaseAugment = new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)

		GenesisTimestamp = Timestamp(1426537000) // Approx. 4:16pm EST Mar. 16th, 2015
		GenesisSiafundUnlockHash = ZeroUnlockHash
		GenesisClaimUnlockHash = ZeroUnlockHash
	}
}
