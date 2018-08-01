package consensus

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/types"

	"github.com/coreos/bbolt"
)

// TestChildTargetOak checks the childTargetOak function, especially for edge
// cases like overflows and underflows.
func TestChildTargetOak(t *testing.T) {
	// NOTE: Test must not be run in parallel.
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	cs := cst.cs
	// NOTE: Test must not be run in parallel.
	//
	// Set the constants to match the real-network constants, and then make sure
	// they are reset at the end of the test.
	oldFreq := types.BlockFrequency
	oldMaxRise := types.OakMaxRise
	oldMaxDrop := types.OakMaxDrop
	oldRootTarget := types.RootTarget
	types.BlockFrequency = 600
	types.OakMaxRise = big.NewRat(1004, 1e3)
	types.OakMaxDrop = big.NewRat(1e3, 1004)
	types.RootTarget = types.Target{0, 0, 0, 1}
	defer func() {
		types.BlockFrequency = oldFreq
		types.OakMaxRise = oldMaxRise
		types.OakMaxDrop = oldMaxDrop
		types.RootTarget = oldRootTarget
	}()

	// Start with some values that are normal, resulting in no change in target.
	parentHeight := types.BlockHeight(100)
	// The total time and total target will be set to 100 uncompressed blocks.
	// Though the actual algorithm is compressing the times to achieve an
	// exponential weighting, this test only requires that the visible hashrate
	// be measured as equal to the root target per block.
	parentTotalTime := int64(types.BlockFrequency * parentHeight)
	parentTotalTarget := types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	parentTimestamp := types.GenesisTimestamp + types.Timestamp((types.BlockFrequency * parentHeight))
	parentTarget := types.RootTarget
	// newTarget should match the root target, as the hashrate and blocktime all
	// match the existing target - there should be no reason for adjustment.
	newTarget := cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight, parentTimestamp)
	// New target should be barely moving. Some imprecision may cause slight
	// adjustments, but the total difference should be less than 0.01%.
	maxNewTarget := parentTarget.MulDifficulty(big.NewRat(10e3, 10001))
	minNewTarget := parentTarget.MulDifficulty(big.NewRat(10001, 10e3))
	if newTarget.Cmp(maxNewTarget) > 0 {
		t.Error("The target shifted too much for a constant hashrate")
	}
	if newTarget.Cmp(minNewTarget) < 0 {
		t.Error("The target shifted too much for a constant hashrate")
	}

	// Set the total time such that the difficulty needs to be adjusted down.
	// Shifter clamps and adjustment clamps will both be in effect.
	parentHeight = types.BlockHeight(100)
	// Set the visible hashrate to types.RootTarget per block.
	parentTotalTime = int64(types.BlockFrequency * parentHeight)
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	// Set the timestamp far in the future, triggering the shifter to increase
	// the block time to the point that the shifter clamps activate.
	parentTimestamp = types.GenesisTimestamp + types.Timestamp((types.BlockFrequency * parentHeight)) + 500e6
	// Set the target to types.RootTarget, causing the max difficulty adjustment
	// clamp to be in effect.
	parentTarget = types.RootTarget
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight, parentTimestamp)
	if parentTarget.Difficulty().Cmp(newTarget.Difficulty()) <= 0 {
		t.Error("Difficulty did not decrease in response to increased total time")
	}
	// Check that the difficulty decreased by the maximum amount.
	minNewTarget = parentTarget.MulDifficulty(types.OakMaxDrop)
	if minNewTarget.Difficulty().Cmp(newTarget.Difficulty()) != 0 {
		t.Error("Difficulty did not decrease by the maximum amount")
	}

	// Set the total time such that the difficulty needs to be adjusted up.
	// Shifter clamps and adjustment clamps will both be in effect.
	parentHeight = types.BlockHeight(100)
	// Set the visible hashrate to types.RootTarget per block.
	parentTotalTime = int64(types.BlockFrequency * parentHeight)
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	// Set the timestamp far in the past, triggering the shifter to decrease the
	// block time to the point that the shifter clamps activate.
	parentTimestamp = types.GenesisTimestamp + types.Timestamp((types.BlockFrequency * parentHeight)) - 500e6
	// Set the target to types.RootTarget, causing the max difficulty adjustment
	// clamp to be in effect.
	parentTarget = types.RootTarget
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight, parentTimestamp)
	if parentTarget.Difficulty().Cmp(newTarget.Difficulty()) >= 0 {
		t.Error("Difficulty did not increase in response to decreased total time")
	}
	// Check that the difficulty decreased by the maximum amount.
	minNewTarget = parentTarget.MulDifficulty(types.OakMaxRise)
	if minNewTarget.Difficulty().Cmp(newTarget.Difficulty()) != 0 {
		t.Error("Difficulty did not increase by the maximum amount")
	}

	// Set the total time such that the difficulty needs to be adjusted down.
	// Neither the shiftor clamps nor the adjustor clamps will be in effect.
	parentHeight = types.BlockHeight(100)
	// Set the visible hashrate to types.RootTarget per block.
	parentTotalTime = int64(types.BlockFrequency * parentHeight)
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	// Set the timestamp in the future, but little enough in the future that
	// neither the shifter clamp nor the adjustment clamp will trigger.
	parentTimestamp = types.GenesisTimestamp + types.Timestamp((types.BlockFrequency * parentHeight)) + 5e3
	// Set the target to types.RootTarget.
	parentTarget = types.RootTarget
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight, parentTimestamp)
	// Check that the difficulty decreased, but not by the max amount.
	minNewTarget = parentTarget.MulDifficulty(types.OakMaxDrop)
	if parentTarget.Difficulty().Cmp(newTarget.Difficulty()) <= 0 {
		t.Error("Difficulty did not decrease")
	}
	if minNewTarget.Difficulty().Cmp(newTarget.Difficulty()) >= 0 {
		t.Error("Difficulty decreased by the clamped amount")
	}

	// Set the total time such that the difficulty needs to be adjusted up.
	// Neither the shiftor clamps nor the adjustor clamps will be in effect.
	parentHeight = types.BlockHeight(100)
	// Set the visible hashrate to types.RootTarget per block.
	parentTotalTime = int64(types.BlockFrequency * parentHeight)
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	// Set the timestamp in the past, but little enough in the future that
	// neither the shifter clamp nor the adjustment clamp will trigger.
	parentTimestamp = types.GenesisTimestamp + types.Timestamp((types.BlockFrequency * parentHeight)) - 5e3
	// Set the target to types.RootTarget.
	parentTarget = types.RootTarget
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight, parentTimestamp)
	// Check that the difficulty increased, but not by the max amount.
	maxNewTarget = parentTarget.MulDifficulty(types.OakMaxRise)
	if parentTarget.Difficulty().Cmp(newTarget.Difficulty()) >= 0 {
		t.Error("Difficulty did not decrease")
	}
	if maxNewTarget.Difficulty().Cmp(newTarget.Difficulty()) <= 0 {
		t.Error("Difficulty decreased by the clamped amount")
	}

	// Set the total time such that the difficulty needs to be adjusted down.
	// Adjustor clamps will be in effect, shifter clamps will not be in effect.
	parentHeight = types.BlockHeight(100)
	// Set the visible hashrate to types.RootTarget per block.
	parentTotalTime = int64(types.BlockFrequency * parentHeight)
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	// Set the timestamp in the future, but little enough in the future that
	// neither the shifter clamp nor the adjustment clamp will trigger.
	parentTimestamp = types.GenesisTimestamp + types.Timestamp((types.BlockFrequency * parentHeight)) + 10e3
	// Set the target to types.RootTarget.
	parentTarget = types.RootTarget
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight, parentTimestamp)
	// Check that the difficulty decreased, but not by the max amount.
	minNewTarget = parentTarget.MulDifficulty(types.OakMaxDrop)
	if parentTarget.Difficulty().Cmp(newTarget.Difficulty()) <= 0 {
		t.Error("Difficulty did not decrease")
	}
	if minNewTarget.Difficulty().Cmp(newTarget.Difficulty()) != 0 {
		t.Error("Difficulty decreased by the clamped amount")
	}

	// Set the total time such that the difficulty needs to be adjusted up.
	// Adjustor clamps will be in effect, shifter clamps will not be in effect.
	parentHeight = types.BlockHeight(100)
	// Set the visible hashrate to types.RootTarget per block.
	parentTotalTime = int64(types.BlockFrequency * parentHeight)
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	// Set the timestamp in the past, but little enough in the future that
	// neither the shifter clamp nor the adjustment clamp will trigger.
	parentTimestamp = types.GenesisTimestamp + types.Timestamp((types.BlockFrequency * parentHeight)) - 10e3
	// Set the target to types.RootTarget.
	parentTarget = types.RootTarget
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight, parentTimestamp)
	// Check that the difficulty increased, but not by the max amount.
	maxNewTarget = parentTarget.MulDifficulty(types.OakMaxRise)
	if parentTarget.Difficulty().Cmp(newTarget.Difficulty()) >= 0 {
		t.Error("Difficulty did not decrease")
	}
	if maxNewTarget.Difficulty().Cmp(newTarget.Difficulty()) != 0 {
		t.Error("Difficulty decreased by the clamped amount")
	}

	// Set the total time such that the difficulty needs to be adjusted down.
	// Shifter clamps will be in effect, adjustor clamps will not be in effect.
	parentHeight = types.BlockHeight(100)
	// Set the visible hashrate to types.RootTarget per block.
	parentTotalTime = int64(types.BlockFrequency * parentHeight)
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	// Set the timestamp in the future, but little enough in the future that
	// neither the shifter clamp nor the adjustment clamp will trigger.
	parentTimestamp = types.GenesisTimestamp + types.Timestamp((types.BlockFrequency * parentHeight)) + 500e6
	// Set the target to types.RootTarget.
	parentTarget = types.RootTarget.MulDifficulty(big.NewRat(1, types.OakMaxBlockShift))
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight, parentTimestamp)
	// New target should be barely moving. Some imprecision may cause slight
	// adjustments, but the total difference should be less than 0.01%.
	maxNewTarget = parentTarget.MulDifficulty(big.NewRat(10e3, 10001))
	minNewTarget = parentTarget.MulDifficulty(big.NewRat(10001, 10e3))
	if newTarget.Cmp(maxNewTarget) > 0 {
		t.Error("The target shifted too much for a constant hashrate")
	}
	if newTarget.Cmp(minNewTarget) < 0 {
		t.Error("The target shifted too much for a constant hashrate")
	}

	// Set the total time such that the difficulty needs to be adjusted up.
	// Shifter clamps will be in effect, adjustor clamps will not be in effect.
	parentHeight = types.BlockHeight(100)
	// Set the visible hashrate to types.RootTarget per block.
	parentTotalTime = int64(types.BlockFrequency * parentHeight)
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	// Set the timestamp in the past, but little enough in the future that
	// neither the shifter clamp nor the adjustment clamp will trigger.
	parentTimestamp = types.GenesisTimestamp + types.Timestamp((types.BlockFrequency * parentHeight)) - 500e6
	// Set the target to types.RootTarget.
	parentTarget = types.RootTarget.MulDifficulty(big.NewRat(types.OakMaxBlockShift, 1))
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight, parentTimestamp)
	// New target should be barely moving. Some imprecision may cause slight
	// adjustments, but the total difference should be less than 0.01%.
	maxNewTarget = parentTarget.MulDifficulty(big.NewRat(10e3, 10001))
	minNewTarget = parentTarget.MulDifficulty(big.NewRat(10001, 10e3))
	if newTarget.Cmp(maxNewTarget) > 0 {
		t.Error("The target shifted too much for a constant hashrate")
	}
	if newTarget.Cmp(minNewTarget) < 0 {
		t.Error("The target shifted too much for a constant hashrate")
	}
}

// TestStoreBlockTotals checks features of the storeBlockTotals and
// getBlockTotals code.
func TestStoreBlockTotals(t *testing.T) {
	// NOTE: Test must not be run in parallel.
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	cs := cst.cs
	// NOTE: Test must not be run in parallel.
	//
	// Set the constants to match the real-network constants, and then make sure
	// they are reset at the end of the test.
	oldFreq := types.BlockFrequency
	oldDecayNum := types.OakDecayNum
	oldDecayDenom := types.OakDecayDenom
	oldMaxRise := types.OakMaxRise
	oldMaxDrop := types.OakMaxDrop
	oldRootTarget := types.RootTarget
	types.BlockFrequency = 600
	types.OakDecayNum = 995
	types.OakDecayDenom = 1e3
	types.OakMaxRise = big.NewRat(1004, 1e3)
	types.OakMaxDrop = big.NewRat(1e3, 1004)
	types.RootTarget = types.Target{0, 0, 0, 1}
	defer func() {
		types.BlockFrequency = oldFreq
		types.OakDecayNum = oldDecayNum
		types.OakDecayDenom = oldDecayDenom
		types.OakMaxRise = oldMaxRise
		types.OakMaxDrop = oldMaxDrop
		types.RootTarget = oldRootTarget
	}()

	// Check that as totals get stored over and over, the values getting
	// returned follow a decay. While storing repeatedly, check that the
	// getBlockTotals values match the values that were stored.
	err = cs.db.Update(func(tx *bolt.Tx) error {
		var totalTime int64
		var id types.BlockID
		var parentTimestamp, currentTimestamp types.Timestamp
		currentTarget := types.RootTarget
		totalTarget := types.RootDepth
		for i := types.BlockHeight(0); i < 8000; i++ {
			id[i/256] = byte(i % 256)
			parentTimestamp = currentTimestamp
			currentTimestamp += types.Timestamp(types.BlockFrequency)
			totalTime, totalTarget, err = cs.storeBlockTotals(tx, i, id, totalTime, parentTimestamp, currentTimestamp, totalTarget, currentTarget)
			if err != nil {
				return err
			}

			// Check that the fetched values match the stored values.
			getTime, getTarg := cs.getBlockTotals(tx, id)
			if getTime != totalTime || getTarg != totalTarget {
				t.Error("fetch failed - retrieving time and target did not work")
			}
		}
		// Do a final iteration, but keep the old totals. After 8000 iterations,
		// the totals should no longer be changing, yet they should be hundreds
		// of times larger than the original values.
		id[8001/256] = byte(8001 % 256)
		parentTimestamp = currentTimestamp
		currentTimestamp += types.Timestamp(types.BlockFrequency)
		newTotalTime, newTotalTarget, err := cs.storeBlockTotals(tx, 8001, id, totalTime, parentTimestamp, currentTimestamp, totalTarget, currentTarget)
		if err != nil {
			return err
		}
		if newTotalTime != totalTime || newTotalTarget.Difficulty().Cmp(totalTarget.Difficulty()) != 0 {
			t.Log(newTotalTime)
			t.Log(totalTime)
			t.Log(newTotalTarget)
			t.Log(totalTarget)
			t.Error("Total time and target did not seem to converge to a result")
		}
		if newTotalTime < int64(types.BlockFrequency)*199 {
			t.Error("decay seems to be happening too rapidly")
		}
		if newTotalTime > int64(types.BlockFrequency)*205 {
			t.Error("decay seems to be happening too slowly")
		}
		if newTotalTarget.Difficulty().Cmp(types.RootTarget.Difficulty().Mul64(199)) < 0 {
			t.Error("decay seems to be happening too rapidly")
		}
		if newTotalTarget.Difficulty().Cmp(types.RootTarget.Difficulty().Mul64(205)) > 0 {
			t.Error("decay seems to be happening too slowly")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestOakHardforkMechanic mines blocks until the oak hardfork kicks in,
// verifying that nothing unusual happens, and that the difficulty adjustments
// begin happening every block.
func TestHardforkMechanic(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()

	// Mine blocks until the oak hardfork height, printing the current target at
	// each height.
	var prevTarg types.Target
	for i := types.BlockHeight(0); i < types.OakHardforkBlock*2; i++ {
		b, err := cst.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		targ, _ := cst.cs.ChildTarget(b.ID())
		if i > types.OakHardforkBlock && bytes.Compare(targ[:], prevTarg[:]) >= 0 {
			t.Error("target is not adjusting down during mining every block")
		}
		prevTarg = targ

	}
}
