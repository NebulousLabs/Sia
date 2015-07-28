package consensus

import (
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestEarliestChildTimestamp probes the earliestChildTimestamp method of the
// block node type.
func TestEarliestChildTimestamp(t *testing.T) {
	// Open a dummy database to store the processedBlocs
	testdir := build.TempDir(modules.ConsensusDir, "TestEarliestChildTimestamp")
	db := openDB(testdir + "/set.db")

	// Check the earliest timestamp generated when the block node has no
	// parent.
	pb1 := &processedBlock{block: types.Block{Timestamp: 1}}
	if pb1.earliestChildTimestamp() != 1 {
		t.Error("earliest child timestamp has been calculated incorrectly.")
	}

	db.addBlockMap(pb1)

	// Set up a series of targets, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11
	pb2 := &processedBlock{block: types.Block{Timestamp: 2}, parent: pb1}
	pb3 := &processedBlock{block: types.Block{Timestamp: 3}, parent: pb2}
	pb4 := &processedBlock{block: types.Block{Timestamp: 4}, parent: pb3}
	pb5 := &processedBlock{block: types.Block{Timestamp: 5}, parent: pb4}
	pb6 := &processedBlock{block: types.Block{Timestamp: 6}, parent: pb5}
	pb7 := &processedBlock{block: types.Block{Timestamp: 7}, parent: pb6}
	pb8 := &processedBlock{block: types.Block{Timestamp: 8}, parent: pb7}
	pb9 := &processedBlock{block: types.Block{Timestamp: 9}, parent: pb8}
	pb10 := &processedBlock{block: types.Block{Timestamp: 10}, parent: pb9}
	pb11 := &processedBlock{block: types.Block{Timestamp: 11}, parent: pb10}

	// Median should be '1' for pb6.
	if pb6.earliestChildTimestamp() != 1 {
		t.Error("incorrect child timestamp")
	}
	// Median should be '2' for pb7.
	if pb7.earliestChildTimestamp() != 2 {
		t.Error("incorrect child timestamp")
	}
	// Median should be '6' for pb11.
	if pb11.earliestChildTimestamp() != 6 {
		t.Error("incorrect child timestamp")
	}

	// Mix up the sorting:
	//           7, 5, 5, 2, 3, 9, 12, 1, 8, 6, 14
	// sorted11: 1, 2, 3, 5, 5, 6, 7, 8, 9, 12, 14
	// sorted10: 1, 2, 3, 5, 5, 6, 7, 7, 8, 9, 12
	// sorted9:  1, 2, 3, 5, 5, 7, 7, 7, 8, 9, 12
	pb1.block.Timestamp = 7
	pb2.block.Timestamp = 5
	pb3.block.Timestamp = 5
	pb4.block.Timestamp = 2
	pb5.block.Timestamp = 3
	pb6.block.Timestamp = 9
	pb7.block.Timestamp = 12
	pb8.block.Timestamp = 1
	pb9.block.Timestamp = 8
	pb10.block.Timestamp = 6
	pb11.block.Timestamp = 14

	// Median of pb11 should be '6'.
	if pb11.earliestChildTimestamp() != 6 {
		t.Error("incorrect child timestamp")
	}
	// Median of pb10 should be '6'.
	if pb10.earliestChildTimestamp() != 6 {
		t.Error("incorrect child timestamp")
	}
	// Median of pb9 should be '7'.
	if pb9.earliestChildTimestamp() != 7 {
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
	// Try adding to equal weight nodes, result should be half.
	bn := new(blockNode)
	bn.depth[0] = 64
	bn.childTarget[0] = 64
	childDepth := bn.childDepth()
	if childDepth[0] != 32 {
		t.Error("unexpected child depth")
	}

	// Try adding nodes of different weights.
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

// TestNewChild probes the newChild method of the block node type.
func TestNewChild(t *testing.T) {
	parent := &blockNode{
		height: 12,
	}
	parent.depth[0] = 45
	parent.block.Timestamp = 100
	parent.childTarget[0] = 90

	child := parent.newChild(types.Block{Timestamp: types.Timestamp(100 + types.BlockFrequency)})
	if child.parent != parent {
		t.Error("parent-child relationship incorrect")
	}
	if child.height != 13 {
		t.Error("child height set incorrectly")
	}
	var expectedDepth types.Target
	expectedDepth[0] = 30
	if child.depth.Cmp(expectedDepth) != 0 {
		t.Error("child depth did not adjust correctly")
	}
	if child.childTarget.Cmp(parent.childTarget) != 0 {
		t.Error("child childTarget not adjusted correctly")
	}
}
