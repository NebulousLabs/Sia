package consensus

import (
	"math/big"
	"os"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestEarliestChildTimestamp probes the earliestChildTimestamp method of the
// block node type.
func TestEarliestChildTimestamp(t *testing.T) {
	// Open a dummy database to store the processedBlocs
	testdir := build.TempDir(modules.ConsensusDir, "TestEarliestChildTimestamp")
	// Create the consensus directory.
	err := os.MkdirAll(testdir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openDB(testdir + "/set.db")
	if err != nil {
		t.Fatal(err)
	}

	// Check the earliest timestamp generated when the block node has no
	// parent.
	pb1 := &processedBlock{Block: types.Block{Timestamp: 1}}
	if pb1.earliestChildTimestamp(db) != 1 {
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
		db.addBlockMap(pb)
	}

	// Median should be '1' for pb6.
	if pb6.earliestChildTimestamp(db) != 1 {
		t.Error("incorrect child timestamp")
	}
	// Median should be '2' for pb7.
	if pb7.earliestChildTimestamp(db) != 2 {
		t.Error("incorrect child timestamp")
	}
	// Median should be '6' for pb11.
	if pb11.earliestChildTimestamp(db) != 6 {
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
	// Open a dummy database to store the processedBlocs
	testdir := build.TempDir(modules.ConsensusDir, "TestTargetAdjustmentBase")
	// Create the consensus directory.
	err := os.MkdirAll(testdir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openDB(testdir + "/set.db")
	if err != nil {
		t.Fatal(err)
	}

	// Create a genesis node at timestamp 10,000
	genesisNode := &processedBlock{
		Block: types.Block{Timestamp: 10000},
	}
	db.addBlockMap(genesisNode)
	exactTimeNode := &processedBlock{
		Block: types.Block{
			Nonce:     types.BlockNonce{1, 0, 0, 0, 0, 0, 0, 0},
			Timestamp: types.Timestamp(10000 + types.BlockFrequency),
		},
	}
	exactTimeNode.Parent = genesisNode.Block.ID()
	db.addBlockMap(exactTimeNode)

	// Base adjustment for the exactTimeNode should be 1.
	adjustment, exact := exactTimeNode.targetAdjustmentBase(db).Float64()
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
	db.addBlockMap(doubleSpeedNode)
	adjustment, exact = doubleSpeedNode.targetAdjustmentBase(db).Float64()
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
	db.addBlockMap(halfSpeedNode)
	adjustment, exact = halfSpeedNode.targetAdjustmentBase(db).Float64()
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
	db.addBlockMap(comparisonNode)
	startingNode := comparisonNode
	for i := types.BlockHeight(0); i < types.TargetWindow; i++ {
		newNode := new(processedBlock)
		newNode.Parent = startingNode.Block.ID()
		newNode.Block.Nonce = types.BlockNonce{byte(i), byte(i / 256), 0, 0, 0, 0, 0, 0}
		db.addBlockMap(newNode)
		startingNode = newNode
	}
	startingNode.Block.Timestamp = types.Timestamp(125000 + types.BlockFrequency*types.TargetWindow)
	adjustment, exact = startingNode.targetAdjustmentBase(db).Float64()
	if !exact {
		t.Error("failed to get exact result")
	}
	if adjustment != 1 {
		t.Error("got wrong long-range adjustment")
	}
	startingNode.Block.Timestamp = types.Timestamp(125000 + 2*types.BlockFrequency*types.TargetWindow)
	adjustment, exact = startingNode.targetAdjustmentBase(db).Float64()
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
	// Open a dummy database to store the processedBlocs
	testdir := build.TempDir(modules.ConsensusDir, "TestSetChildTarget")
	// Create the consensus directory.
	err := os.MkdirAll(testdir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openDB(testdir + "/set.db")
	if err != nil {
		t.Fatal(err)
	}

	// Create a genesis node and a child that took 2x as long as expected.
	genesisNode := &processedBlock{
		Block: types.Block{Timestamp: 10000},
	}
	genesisNode.ChildTarget[0] = 64
	db.addBlockMap(genesisNode)
	doubleTimeNode := &processedBlock{
		Block: types.Block{Timestamp: types.Timestamp(10000 + types.BlockFrequency*2)},
	}
	doubleTimeNode.Parent = genesisNode.Block.ID()
	db.addBlockMap(doubleTimeNode)

	// Check the resulting childTarget of the new node and see that the clamp
	// was applied.
	doubleTimeNode.setChildTarget(db)
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
	// Open a dummy database to store the processedBlocs
	testdir := build.TempDir(modules.ConsensusDir, "TestSetChildTarget")
	// Create the consensus directory.
	err := os.MkdirAll(testdir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openDB(testdir + "/set.db")
	if err != nil {
		t.Fatal(err)
	}

	parent := &processedBlock{
		Height: 12,
	}
	parent.Depth[0] = 45
	parent.Block.Timestamp = 100
	parent.ChildTarget[0] = 90

	db.addBlockMap(parent)

	child := parent.newChild(types.Block{Timestamp: types.Timestamp(100 + types.BlockFrequency)}, db)
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
