package consensus

import (
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestEarliestChildTimestamp probes the earliestChildTimestamp method of the
// block node type.
func TestEarliestChildTimestamp(t *testing.T) {
	cst, err := createConsensusSetTester("TestEarliestChildTimestamp")
	if err != nil {
		t.Fatal(err)
	}

	// Check the earliest timestamp generated when the block node has no
	// parent.
	pb1 := &processedBlock{Block: types.Block{Timestamp: 1}}
	if cst.cs.earliestChildTimestamp(pb1) != 1 {
		t.Error("earliest child timestamp has been calculated incorrectly.")
	}

	// Set up a series of targets, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11
	pb2 := &processedBlock{Block: types.Block{Timestamp: 2}, Parent: pb1.Block.ID()}
	pb3 := &processedBlock{Block: types.Block{Timestamp: 3}, Parent: pb2.Block.ID()}
	pb4 := &processedBlock{Block: types.Block{Timestamp: 4}, Parent: pb3.Block.ID()}
	pb5 := &processedBlock{Block: types.Block{Timestamp: 5}, Parent: pb4.Block.ID()}
	pb6 := &processedBlock{Block: types.Block{Timestamp: 6}, Parent: pb5.Block.ID()}
	pb7 := &processedBlock{Block: types.Block{Timestamp: 7}, Parent: pb6.Block.ID()}
	pb8 := &processedBlock{Block: types.Block{Timestamp: 8}, Parent: pb7.Block.ID()}
	pb9 := &processedBlock{Block: types.Block{Timestamp: 9}, Parent: pb8.Block.ID()}
	pb10 := &processedBlock{Block: types.Block{Timestamp: 10}, Parent: pb9.Block.ID()}
	pb11 := &processedBlock{Block: types.Block{Timestamp: 11}, Parent: pb10.Block.ID()}

	pbs := []*processedBlock{pb1, pb2, pb3, pb4, pb5, pb6, pb7, pb8, pb9, pb10, pb11}
	for _, pb := range pbs {
		cst.cs.db.addBlockMap(pb)
	}

	// Median should be '1' for pb6.
	if cst.cs.earliestChildTimestamp(pb6) != 1 {
		t.Error("incorrect child timestamp")
	}
	// Median should be '2' for pb7.
	if cst.cs.earliestChildTimestamp(pb7) != 2 {
		t.Error("incorrect child timestamp")
	}
	// Median should be '6' for pb11.
	if cst.cs.earliestChildTimestamp(pb11) != 6 {
		t.Error("incorrect child timestamp")
	}
}

// TestHeavierThan probes the heavierThan method of the processedBlock type.
func TestHeavierThan(t *testing.T) {
	// Create a light node.
	pbLight := new(processedBlock)
	pbLight.Depth[0] = 64
	pbLight.ChildTarget[0] = 200

	// Create a node that's heavier, but not enough to beat the surpass
	// threshold.
	pbMiddle := new(processedBlock)
	pbMiddle.Depth[0] = 60
	pbMiddle.ChildTarget[0] = 200

	// Create a node that's heavy enough to break the surpass threshold.
	pbHeavy := new(processedBlock)
	pbHeavy.Depth[0] = 16
	pbHeavy.ChildTarget[0] = 200

	// pbLight should not be heavier than pbHeavy.
	if pbLight.heavierThan(pbHeavy) {
		t.Error("light heavier than heavy")
	}
	// pbLight should not be heavier than middle.
	if pbLight.heavierThan(pbMiddle) {
		t.Error("light heavier than middle")
	}
	// pbLight should not be heavier than itself.
	if pbLight.heavierThan(pbLight) {
		t.Error("light heavier than itself")
	}

	// pbMiddle should not be heavier than pbLight.
	if pbMiddle.heavierThan(pbLight) {
		t.Error("middle heaver than light - surpass threshold should not have been broken")
	}
	// pbHeavy should be heaver than pbLight.
	if !pbHeavy.heavierThan(pbLight) {
		t.Error("heavy is not heavier than light")
	}
	// pbHeavy should be heavier than pbMiddle.
	if !pbHeavy.heavierThan(pbMiddle) {
		t.Error("heavy is not heavier than middle")
	}
}

// TestChildDepth probes the childDeath method of the blockNode type.
func TestChildDept(t *testing.T) {
	// Try adding to equal weight nodes, result should be half.
	pb := new(processedBlock)
	pb.Depth[0] = 64
	pb.ChildTarget[0] = 64
	childDepth := pb.childDepth()
	if childDepth[0] != 32 {
		t.Error("unexpected child depth")
	}

	// Try adding nodes of different weights.
	pb.Depth[0] = 24
	pb.ChildTarget[0] = 48
	childDepth = pb.childDepth()
	if childDepth[0] != 16 {
		t.Error("unexpected child depth")
	}
}

// TestTargetAdjustmentBase probes the targetAdjustmentBase method of the block
// node type.
func TestTargetAdjustmentBase(t *testing.T) {
	cst, err := createConsensusSetTester("TestTargetAdjustmentBase")
	if err != nil {
		t.Fatal(err)
	}

	// Create a genesis node at timestamp 10,000
	genesisNode := &processedBlock{
		Block: types.Block{Timestamp: 10000},
	}
	cst.cs.db.addBlockMap(genesisNode)
	exactTimeNode := &processedBlock{
		Block: types.Block{
			Nonce:     types.BlockNonce{1, 0, 0, 0, 0, 0, 0, 0},
			Timestamp: types.Timestamp(10000 + types.BlockFrequency),
		},
	}
	exactTimeNode.Parent = genesisNode.Block.ID()
	cst.cs.db.addBlockMap(exactTimeNode)

	// Base adjustment for the exactTimeNode should be 1.
	adjustment, exact := cst.cs.targetAdjustmentBase(exactTimeNode).Float64()
	if !exact {
		t.Fatal("did not get an exact target adjustment")
	}
	if adjustment != 1 {
		t.Error("block did not adjust itself to the same target")
	}

	// Create a double-speed node and get the base adjustment.
	doubleSpeedNode := &processedBlock{
		Block: types.Block{Timestamp: types.Timestamp(10000 + types.BlockFrequency)},
	}
	doubleSpeedNode.Parent = exactTimeNode.Block.ID()
	cst.cs.db.addBlockMap(doubleSpeedNode)
	adjustment, exact = cst.cs.targetAdjustmentBase(doubleSpeedNode).Float64()
	if !exact {
		t.Fatal("did not get an exact adjustment")
	}
	if adjustment != 0.5 {
		t.Error("double speed node did not get a base to halve the target")
	}

	// Create a half-speed node and get the base adjustment.
	halfSpeedNode := &processedBlock{
		Block: types.Block{Timestamp: types.Timestamp(10000 + types.BlockFrequency*6)},
	}
	halfSpeedNode.Parent = doubleSpeedNode.Block.ID()
	cst.cs.db.addBlockMap(halfSpeedNode)
	adjustment, exact = cst.cs.targetAdjustmentBase(halfSpeedNode).Float64()
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
	comparisonNode := &processedBlock{
		Block: types.Block{Timestamp: 125000},
	}
	comparisonNode.Parent = halfSpeedNode.Block.ID()
	cst.cs.db.addBlockMap(comparisonNode)
	startingNode := comparisonNode
	for i := types.BlockHeight(0); i < types.TargetWindow; i++ {
		newNode := new(processedBlock)
		newNode.Parent = startingNode.Block.ID()
		newNode.Block.Nonce = types.BlockNonce{byte(i), byte(i / 256), 0, 0, 0, 0, 0, 0}
		cst.cs.db.addBlockMap(newNode)
		startingNode = newNode
	}
	startingNode.Block.Timestamp = types.Timestamp(125000 + types.BlockFrequency*types.TargetWindow)
	adjustment, exact = cst.cs.targetAdjustmentBase(startingNode).Float64()
	if !exact {
		t.Error("failed to get exact result")
	}
	if adjustment != 1 {
		t.Error("got wrong long-range adjustment")
	}
	startingNode.Block.Timestamp = types.Timestamp(125000 + 2*types.BlockFrequency*types.TargetWindow)
	adjustment, exact = cst.cs.targetAdjustmentBase(startingNode).Float64()
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
	cst, err := createConsensusSetTester("TestSetChildTarget")
	if err != nil {
		t.Fatal(err)
	}

	// Create a genesis node and a child that took 2x as long as expected.
	genesisNode := &processedBlock{
		Block: types.Block{Timestamp: 10000},
	}
	genesisNode.ChildTarget[0] = 64
	cst.cs.db.addBlockMap(genesisNode)
	doubleTimeNode := &processedBlock{
		Block: types.Block{Timestamp: types.Timestamp(10000 + types.BlockFrequency*2)},
	}
	doubleTimeNode.Parent = genesisNode.Block.ID()
	cst.cs.db.addBlockMap(doubleTimeNode)

	// Check the resulting childTarget of the new node and see that the clamp
	// was applied.
	cst.cs.setChildTarget(doubleTimeNode)
	if doubleTimeNode.ChildTarget.Cmp(genesisNode.ChildTarget) <= 0 {
		t.Error("double time node target did not increase")
	}
	fullAdjustment := genesisNode.ChildTarget.MulDifficulty(big.NewRat(1, 2))
	if doubleTimeNode.ChildTarget.Cmp(fullAdjustment) >= 0 {
		t.Error("clamp was not applied when adjusting target")
	}
}

// TestNewChild probes the newChild method of the block node type.
func TestNewChild(t *testing.T) {
	cst, err := createConsensusSetTester("TestSetChildTarget")
	if err != nil {
		t.Fatal(err)
	}

	parent := &processedBlock{
		Height: 12,
	}
	parent.Depth[0] = 45
	parent.Block.Timestamp = 100
	parent.ChildTarget[0] = 90

	cst.cs.db.addBlockMap(parent)

	child := cst.cs.newChild(parent, types.Block{Timestamp: types.Timestamp(100 + types.BlockFrequency)})
	if child.Parent != parent.Block.ID() {
		t.Error("parent-child relationship incorrect")
	}
	if child.Height != 13 {
		t.Error("child height set incorrectly")
	}
	var expectedDepth types.Target
	expectedDepth[0] = 30
	if child.Depth.Cmp(expectedDepth) != 0 {
		t.Error("child depth did not adjust correctly")
	}
	if child.ChildTarget.Cmp(parent.ChildTarget) != 0 {
		t.Error("child childTarget not adjusted correctly")
	}
}
