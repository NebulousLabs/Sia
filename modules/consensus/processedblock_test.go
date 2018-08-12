package consensus

import (
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationMinimumValidChildTimestamp probes the
// MinimumValidChildTimestamp method of the consensus type.
func TestIntegrationMinimumValidChildTimestamp(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create a custom consensus set to control the blocks.
	testdir := build.TempDir(modules.ConsensusDir, t.Name())
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		t.Fatal(err)
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		t.Fatal(err)
	}
	key := crypto.GenerateTwofishKey()
	_, err = w.Encrypt(key)
	if err != nil {
		t.Fatal(err)
	}
	err = w.Unlock(key)
	if err != nil {
		t.Fatal(err)
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	// The earliest child timestamp of the genesis block should be the
	// timestamp of the genesis block.
	genesisTime := cs.blockRoot.Block.Timestamp
	earliest, ok := cs.MinimumValidChildTimestamp(cs.blockRoot.Block.ID())
	if !ok || genesisTime != earliest {
		t.Error("genesis block earliest timestamp producing unexpected results")
	}

	timestampOffsets := []types.Timestamp{1, 3, 2, 5, 4, 6, 7, 8, 9, 10}
	blockIDs := []types.BlockID{cs.blockRoot.Block.ID()}
	for _, offset := range timestampOffsets {
		bfw, target, err := m.BlockForWork()
		if err != nil {
			t.Fatal(err)
		}
		bfw.Timestamp = genesisTime + offset
		solvedBlock, _ := m.SolveBlock(bfw, target)
		err = cs.AcceptBlock(solvedBlock)
		if err != nil {
			t.Fatal(err)
		}
		blockIDs = append(blockIDs, solvedBlock.ID())
	}

	// Median should be genesisTime for 6th block.
	earliest, ok = cs.MinimumValidChildTimestamp(blockIDs[5])
	if !ok || earliest != genesisTime {
		t.Error("incorrect child timestamp")
	}
	// Median should be genesisTime+1 for 7th block.
	earliest, ok = cs.MinimumValidChildTimestamp(blockIDs[6])
	if !ok || earliest != genesisTime+1 {
		t.Error("incorrect child timestamp")
	}
	// Median should be genesisTime + 5 for pb11.
	earliest, ok = cs.MinimumValidChildTimestamp(blockIDs[10])
	if !ok || earliest != genesisTime+5 {
		t.Error("incorrect child timestamp")
	}
}

// TestUnitHeavierThan probes the heavierThan method of the processedBlock type.
func TestUnitHeavierThan(t *testing.T) {
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
func TestChildDepth(t *testing.T) {
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

/*
// TestTargetAdjustmentBase probes the targetAdjustmentBase method of the block
// node type.
func TestTargetAdjustmentBase(t *testing.T) {
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

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
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

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
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.closeCst()

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
*/
