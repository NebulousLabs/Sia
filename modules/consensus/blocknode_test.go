package consensus

import (
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestEarliestChildTimestamp probes the earliestChildTimestamp method of the
// block node type.
func TestEarliestChildTimestamp(t *testing.T) {
	// Check the earliest timestamp generated when the block node has no
	// parent.
	bn1 := &blockNode{block: types.Block{Timestamp: 1}}
	if bn1.earliestChildTimestamp() != 1 {
		t.Error("earliest child timestamp has been calculated incorrectly.")
	}

	// Set up a series of targets, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11
	bn2 := &blockNode{block: types.Block{Timestamp: 2}, parent: bn1}
	bn3 := &blockNode{block: types.Block{Timestamp: 3}, parent: bn2}
	bn4 := &blockNode{block: types.Block{Timestamp: 4}, parent: bn3}
	bn5 := &blockNode{block: types.Block{Timestamp: 5}, parent: bn4}
	bn6 := &blockNode{block: types.Block{Timestamp: 6}, parent: bn5}
	bn7 := &blockNode{block: types.Block{Timestamp: 7}, parent: bn6}
	bn8 := &blockNode{block: types.Block{Timestamp: 8}, parent: bn7}
	bn9 := &blockNode{block: types.Block{Timestamp: 9}, parent: bn8}
	bn10 := &blockNode{block: types.Block{Timestamp: 10}, parent: bn9}
	bn11 := &blockNode{block: types.Block{Timestamp: 11}, parent: bn10}

	// Median should be '1' for bn6.
	if bn6.earliestChildTimestamp() != 1 {
		t.Error("incorrect child timestamp")
	}
	// Median should be '2' for bn7.
	if bn7.earliestChildTimestamp() != 2 {
		t.Error("incorrect child timestamp")
	}
	// Median should be '6' for bn11.
	if bn11.earliestChildTimestamp() != 6 {
		t.Error("incorrect child timestamp")
	}

	// Mix up the sorting:
	//           7, 5, 5, 2, 3, 9, 12, 1, 8, 6, 14
	// sorted11: 1, 2, 3, 5, 5, 6, 7, 8, 9, 12, 14
	// sorted10: 1, 2, 3, 5, 5, 6, 7, 7, 8, 9, 12
	// sorted9:  1, 2, 3, 5, 5, 7, 7, 7, 8, 9, 12
	bn1.block.Timestamp = 7
	bn2.block.Timestamp = 5
	bn3.block.Timestamp = 5
	bn4.block.Timestamp = 2
	bn5.block.Timestamp = 3
	bn6.block.Timestamp = 9
	bn7.block.Timestamp = 12
	bn8.block.Timestamp = 1
	bn9.block.Timestamp = 8
	bn10.block.Timestamp = 6
	bn11.block.Timestamp = 14

	// Median of bn11 should be '6'.
	if bn11.earliestChildTimestamp() != 6 {
		t.Error("incorrect child timestamp")
	}
	// Median of bn10 should be '6'.
	if bn10.earliestChildTimestamp() != 6 {
		t.Error("incorrect child timestamp")
	}
	// Median of bn9 should be '7'.
	if bn9.earliestChildTimestamp() != 7 {
		t.Error("incorrect child timestamp")
	}
}

// TestHeavierThan probes the heavierThan method of the blockNode type.
func TestHeavierThan(t *testing.T) {
	// Create a light node.
	bnLight := new(blockNode)
	bnLight.depth[0] = 64
	bnLight.childTarget[0] = 200

	// Create a node that's heavier, but not enough to beat the surpass
	// threshold.
	bnMiddle := new(blockNode)
	bnMiddle.depth[0] = 60
	bnMiddle.childTarget[0] = 200

	// Create a node that's heavy enough to break the surpass threshold.
	bnHeavy := new(blockNode)
	bnHeavy.depth[0] = 16
	bnHeavy.childTarget[0] = 200

	// bnLight should not be heavier than bnHeavy.
	if bnLight.heavierThan(bnHeavy) {
		t.Error("light heavier than heavy")
	}
	// bnLight should not be heavier than middle.
	if bnLight.heavierThan(bnMiddle) {
		t.Error("light heavier than middle")
	}
	// bnLight should not be heavier than itself.
	if bnLight.heavierThan(bnLight) {
		t.Error("light heavier than itself")
	}

	// bnMiddle should not be heavier than bnLight.
	if bnMiddle.heavierThan(bnLight) {
		t.Error("middle heaver than light - surpass threshold should not have been broken")
	}
	// bnHeavy should be heaver than bnLight.
	if !bnHeavy.heavierThan(bnLight) {
		t.Error("heavy is not heavier than light")
	}
	// bnHeavy should be heavier than bnMiddle.
	if !bnHeavy.heavierThan(bnMiddle) {
		t.Error("heavy is not heavier than middle")
	}
}

// TestChildDepth probes the childDeath method of the blockNode type.
func TestChildDept(t *testing.T) {
	bn := new(blockNode)
	bn.depth[0] = 64
	bn.childTarget[0] = 64
	childDepth := bn.childDepth()
	if childDepth[0] != 32 {
		t.Error("unexpected child depth")
	}

	bn.depth[0] = 24
	bn.childTarget[0] = 48
	childDepth = bn.childDepth()
	if childDepth[0] != 16 {
		t.Error("unexpected child depth")
	}
}

// TestTargetAdjustmentBase probes the targetAdjustmentBase method of the block
// node type.
func TestTargetAdjustmentBase(t *testing.T) {
	// Create a genesis node at timestamp 10,000
	genesisNode := &blockNode{
		block: types.Block{Timestamp: 10000},
	}
	exactTimeNode := &blockNode{
		block: types.Block{Timestamp: types.Timestamp(10000 + types.BlockFrequency)},
	}
	exactTimeNode.parent = genesisNode

	// Base adjustment for the exactTimeNode should be 1.
	adjustment, exact := exactTimeNode.targetAdjustmentBase().Float64()
	if !exact {
		t.Fatal("did not get an exact target adjustment")
	}
	if adjustment != 1 {
		t.Error("block did not adjust itself to the same target")
	}

	// Create a double-speed node and get the base adjustment.
	doubleSpeedNode := &blockNode{
		block: types.Block{Timestamp: types.Timestamp(10000 + types.BlockFrequency)},
	}
	doubleSpeedNode.parent = exactTimeNode
	adjustment, exact = doubleSpeedNode.targetAdjustmentBase().Float64()
	if !exact {
		t.Fatal("did not get an exact adjustment")
	}
	if adjustment != 0.5 {
		t.Error("double speed node did not get a base to halve the target")
	}

	// Create a half-speed node and get the base adjustment.
	halfSpeedNode := &blockNode{
		block: types.Block{Timestamp: types.Timestamp(10000 + types.BlockFrequency*6)},
	}
	halfSpeedNode.parent = doubleSpeedNode
	adjustment, exact = halfSpeedNode.targetAdjustmentBase().Float64()
	if !exact {
		t.Fatal("did not get an exact adjustment")
	}
	if adjustment != 2 {
		t.Error("double speed node did not get a base to halve the target")
	}

	if testing.Short() {
		t.SkipNow()
	}
	// Create a chain of nodes so that the genesis node is no longer the point
	// of comparison.
	comparisonNode := &blockNode{
		block: types.Block{Timestamp: 125000},
	}
	comparisonNode.parent = halfSpeedNode
	startingNode := comparisonNode
	for i := types.BlockHeight(0); i < types.TargetWindow; i++ {
		newNode := new(blockNode)
		newNode.parent = startingNode
		startingNode = newNode
	}
	startingNode.block.Timestamp = types.Timestamp(125000 + types.BlockFrequency*types.TargetWindow)
	adjustment, exact = startingNode.targetAdjustmentBase().Float64()
	if !exact {
		t.Error("failed to get exact result")
	}
	if adjustment != 1 {
		t.Error("got wrong long-range adjustment")
	}
	startingNode.block.Timestamp = types.Timestamp(125000 + 2*types.BlockFrequency*types.TargetWindow)
	adjustment, exact = startingNode.targetAdjustmentBase().Float64()
	if !exact {
		t.Error("failed to get exact result")
	}
	if adjustment != 2 {
		t.Error("got wrong long-range adjustment")
	}
}

// TestClampTargetAdjustment probes the clampTargetAdjustment function.
func TestClampTargetAdjustment(t *testing.T) {
	// Check that the MaxAdjustmentUp and MaxAdjustmentDown constants match the
	// test's expectations.
	if types.MaxAdjustmentUp.Cmp(big.NewRat(10001, 10000)) != 0 {
		t.Fatal("MaxAdjustmentUp changed - test now invalid")
	}
	if types.MaxAdjustmentDown.Cmp(big.NewRat(9999, 10000)) != 0 {
		t.Fatal("MaxAdjustmentDown changed - test now invalid")
	}

	// Check high and low clamping.
	initial := big.NewRat(2, 1)
	clamped := clampTargetAdjustment(initial)
	if clamped.Cmp(big.NewRat(10001, 10000)) != 0 {
		t.Error("clamp not applied to large target adjustment")
	}
	initial = big.NewRat(1, 2)
	clamped = clampTargetAdjustment(initial)
	if clamped.Cmp(big.NewRat(9999, 10000)) != 0 {
		t.Error("clamp not applied to small target adjustment")
	}

	// Check middle clamping (or lack thereof).
	initial = big.NewRat(10002, 10001)
	clamped = clampTargetAdjustment(initial)
	if clamped.Cmp(initial) != 0 {
		t.Error("clamp applied to safe target adjustment")
	}
	initial = big.NewRat(99999, 100000)
	clamped = clampTargetAdjustment(initial)
	if clamped.Cmp(initial) != 0 {
		t.Error("clamp applied to safe target adjustment")
	}
}

// TestSetChildTarget probes the setChildTarget method of the block node type.
func TestSetChildTarget(t *testing.T) {
	// Create a genesis node and a child that took 2x as long as expected.
	genesisNode := &blockNode{
		block: types.Block{Timestamp: 10000},
	}
	genesisNode.childTarget[0] = 64
	doubleTimeNode := &blockNode{
		block: types.Block{Timestamp: types.Timestamp(10000 + types.BlockFrequency*2)},
	}
	doubleTimeNode.parent = genesisNode

	// Check the resulting childTarget of the new node and see that the clamp
	// was applied.
	doubleTimeNode.setChildTarget()
	if doubleTimeNode.childTarget.Cmp(genesisNode.childTarget) <= 0 {
		t.Error("double time node target did not increase")
	}
	fullAdjustment := genesisNode.childTarget.MulDifficulty(big.NewRat(1, 2))
	if doubleTimeNode.childTarget.Cmp(fullAdjustment) >= 0 {
		t.Error("clamp was not applied when adjusting target")
	}
}
