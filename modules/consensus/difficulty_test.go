package consensus

import (
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestChildTargetOak checks the childTargetOak function, espeically for edge
// cases like overflows and underflows.
func TestChildTargetOak(t *testing.T) {
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

	// Empty/dud consensus set used to call the childTargetOak function.
	//
	// TODO: Replace with a blank consensus set tester.
	cs := new(ConsensusSet)

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
