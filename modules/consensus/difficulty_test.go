package consensus

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
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
	parentTotalTime := int64(types.BlockFrequency * parentHeight)
	parentTotalTarget := types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	parentTarget := types.RootTarget
	// newTarget should match the root target, as the hashrate and blocktime all
	// match the existing target - there should be no reason for adjustment.
	newTarget := cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight)
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

	// Probe the target clamps and the block deltas by providing a correct
	// hashrate, but a parent total time that is very far in the future, which
	// means that blocks have been taking too long - this means that the target
	// block time should be decreased, the difficulty should go down (and target
	// up).
	parentHeight = types.BlockHeight(100)
	parentTotalTime = int64(types.BlockFrequency*parentHeight) + 500e6 // very large delta used to probe extremes
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	parentTarget = types.RootTarget
	// newTarget should be higher, representing reduced difficulty. It should be
	// as high as the adjustment clamp allows it to move.
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight)
	expectedTarget := parentTarget.MulDifficulty(types.OakMaxDrop)
	if newTarget.Cmp(expectedTarget) != 0 {
		t.Log(parentTarget)
		t.Log(expectedTarget)
		t.Log(newTarget)
		t.Error("target was not adjusted correctly when the block delta was put to an extreme")
	}
	// Check that the difficulty decreased from the parent.
	if newTarget.Difficulty().Cmp(parentTarget.Difficulty()) >= 0 {
		t.Log(newTarget.Difficulty())
		t.Log(expectedTarget.Difficulty())
		t.Error("difficulty has risen when we need the block time to be shorter")
	}

	// Use the same values as the previous check, but set the parent target so
	// it's within range (but above) the adjustment, so the clamp is not
	// triggered.
	parentHeight = types.BlockHeight(100)
	parentTotalTime = int64(types.BlockFrequency*parentHeight) + 500e6
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	parentTarget = types.Target{0, 0, 97, 120}
	// New target should be higher, but the adjustment clamp should not have
	// kicked in.
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight)
	minNewTarget = parentTarget.MulDifficulty(types.OakMaxDrop)
	// Check that the difficulty of the new target decreased.
	if parentTarget.Difficulty().Cmp(newTarget.Difficulty()) <= 0 {
		t.Error("Difficulty did not decrease")
	}
	// Check that the difficulty decreased by less than the clamped amount.
	if minNewTarget.Difficulty().Cmp(newTarget.Difficulty()) >= 0 {
		t.Error("Difficulty decreased by too much - clamp should not be in effect for these values")
	}

	// A repeat of the second test, except that blocks are coming out too fast
	// instead of too slow, meaning we should see an increased difficulty and a
	// slower block time.
	parentHeight = types.BlockHeight(10e3)
	parentTotalTime = int64(100)
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	parentTarget = types.RootTarget
	// newTarget should be lower, representing increased difficulty. It should
	// be as high as the adjustment clamp allows it to move.
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight)
	expectedTarget = parentTarget.MulDifficulty(types.OakMaxRise)
	if newTarget.Cmp(expectedTarget) != 0 {
		t.Log(parentTarget)
		t.Log(expectedTarget)
		t.Log(newTarget)
		t.Error("target was not adjusted correctly when the block delta was put to an extreme")
	}
	// Check that the difficulty increased from the parent.
	if newTarget.Difficulty().Cmp(parentTarget.Difficulty()) <= 0 {
		t.Log(newTarget.Difficulty())
		t.Log(expectedTarget.Difficulty())
		t.Error("difficulty has dropped when we need the block time to be longer")
	}

	// Use the same values as the previous check, but set the parent target so
	// it's within range (but below) the adjustment, so the clamp is not
	// triggered.
	parentHeight = types.BlockHeight(10e3)
	parentTotalTime = int64(100)
	parentTotalTarget = types.RootTarget.MulDifficulty(big.NewRat(int64(parentHeight), 1))
	parentTarget = types.Target{0, 0, 0, 0, 0, 0, 93, 70}
	// New target should be higher, but the adjustment clamp should not have
	// kicked in.
	newTarget = cs.childTargetOak(parentTotalTime, parentTotalTarget, parentTarget, parentHeight)
	minNewTarget = parentTarget.MulDifficulty(types.OakMaxRise)
	// Check that the difficulty of the new target decreased.
	if parentTarget.Difficulty().Cmp(newTarget.Difficulty()) >= 0 {
		t.Error("Difficulty did not increase")
	}
	// Check that the difficulty decreased by less than the clamped amount.
	if minNewTarget.Difficulty().Cmp(newTarget.Difficulty()) <= 0 {
		t.Error("Difficulty increased by too much - clamp should not be in effect for these values")
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
		totalTime := int64(0)
		totalTarget := types.RootDepth
		var id types.BlockID
		parentTimestamp := types.Timestamp(0)
		currentTimestamp := types.Timestamp(0)
		currentTarget := types.RootTarget
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
