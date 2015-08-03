package consensus

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"math/big"
	"sort"
)

// SurpassThreshold is a percentage that dictates how much heavier ac ompeting
// chain has to be before the node will switch to mining on that chain. This is
// not a consensus rule. This percentage is only applied to the most recent
// block, not the entire chain; see blockNode.heavierThan.
//
// If no threshold were in place, it would be possible to manipulate a block's
// timestamp to produce a sufficiently heavier block.
var SurpassThreshold = big.NewRat(20, 100)

// processedBlock is a copy/rename of blockNode, with the pointers to
// other blockNodes replaced with block ID's, and all the fields
// exported, so that a block node can be marshalled
type processedBlock struct {
	Block    types.Block
	Parent   types.BlockID
	Children []types.BlockID

	Height      types.BlockHeight
	Depth       types.Target
	ChildTarget types.Target

	DiffsGenerated            bool
	SiacoinOutputDiffs        []modules.SiacoinOutputDiff
	FileContractDiffs         []modules.FileContractDiff
	SiafundOutputDiffs        []modules.SiafundOutputDiff
	DelayedSiacoinOutputDiffs []modules.DelayedSiacoinOutputDiff
	SiafundPoolDiffs          []modules.SiafundPoolDiff

	ConsensusSetHash crypto.Hash
}

// earliestChildTimestamp returns the earliest timestamp that a child node
// can have while still being valid. See section 'Timestamp Rules' in
// Consensus.md.
func (pb *processedBlock) earliestChildTimestamp(db *setDB) types.Timestamp {
	// Get the previous MedianTimestampWindow timestamps.
	windowTimes := make(types.TimestampSlice, types.MedianTimestampWindow)
	current := pb
	for i := uint64(0); i < types.MedianTimestampWindow; i++ {
		windowTimes[i] = current.Block.Timestamp

		// If we are at the genesis block, keep using the genesis block for the
		// remaining times.
		if current.Parent != types.ZeroID {
			current = db.getBlockMap(current.Parent)
		}
	}
	sort.Sort(windowTimes)

	// Return the median of the sorted timestamps.
	return windowTimes[len(windowTimes)/2]
}

// heavierThan returns true if the blockNode is sufficiently heavier than
// 'cmp'. 'cmp' is expected to be the current block node. "Sufficient" means
// that the weight of 'bn' exceeds the weight of 'cmp' by:
//		(the target of 'cmp' * 'Surpass Threshold')
func (pb *processedBlock) heavierThan(cmp *processedBlock) bool {
	requirement := cmp.Depth.AddDifficulties(cmp.ChildTarget.MulDifficulty(SurpassThreshold))
	return requirement.Cmp(pb.Depth) > 0 // Inversed, because the smaller target is actually heavier.
}

// childDepth returns the depth of a blockNode's child nodes. The depth is the
// "sum" of the current depth and current difficulty. See target.Add for more
// detailed information.
func (pb *processedBlock) childDepth() types.Target {
	return pb.Depth.AddDifficulties(pb.ChildTarget)
}

// targetAdjustmentBase returns the magnitude that the target should be
// adjusted by before a clamp is applied.
func (pb *processedBlock) targetAdjustmentBase(db *setDB) *big.Rat {
	// Target only adjusts twice per window.
	if pb.Height%(types.TargetWindow/2) != 0 {
		return big.NewRat(1, 1)
	}

	// Grab the block that was generated 'TargetWindow' blocks prior to the
	// parent. If there are not 'TargetWindow' blocks yet, stop at the genesis
	// block.
	var windowSize types.BlockHeight
	windowStart := pb
	for windowSize = 0; windowSize < types.TargetWindow && windowStart.Parent != types.ZeroID; windowSize++ {
		windowStart = db.getBlockMap(windowStart.Parent)
	}

	// The target of a child is determined by the amount of time that has
	// passed between the generation of its immediate parent and its
	// TargetWindow'th parent. The expected amount of seconds to have passed is
	// TargetWindow*BlockFrequency. The target is adjusted in proportion to how
	// time has passed vs. the expected amount of time to have passed.
	//
	// The target is converted to a big.Rat to provide infinite precision
	// during the calculation. The big.Rat is just the int representation of a
	// target.
	timePassed := pb.Block.Timestamp - windowStart.Block.Timestamp
	expectedTimePassed := types.BlockFrequency * windowSize
	return big.NewRat(int64(timePassed), int64(expectedTimePassed))
}

// setChildTarget computes the target of a blockNode's child. All children of a node
// have the same target.
func (pb *processedBlock) setChildTarget(db *setDB) {
	adjustment := clampTargetAdjustment(pb.targetAdjustmentBase(db))
	parent := db.getBlockMap(pb.Parent)
	adjustedRatTarget := new(big.Rat).Mul(parent.ChildTarget.Rat(), adjustment)
	pb.ChildTarget = types.RatToTarget(adjustedRatTarget)
}

// clampTargetAdjustment returns a clamped version of the base adjustment
// value. The clamp keeps the maximum adjustment to ~7x every 2000 blocks. This
// ensures that raising and lowering the difficulty requires a minimum amount
// of total work, which prevents certain classes of difficulty adjusting
// attacks.
func clampTargetAdjustment(base *big.Rat) *big.Rat {
	if base.Cmp(types.MaxAdjustmentUp) > 0 {
		return types.MaxAdjustmentUp
	} else if base.Cmp(types.MaxAdjustmentDown) < 0 {
		return types.MaxAdjustmentDown
	}
	return base
}

// newChild creates a blockNode from a block and adds it to the parent's set of
// children. The new node is also returned. It necessairly modifies the database
func (pb *processedBlock) newChild(b types.Block, db *setDB) *processedBlock {
	// Create the child node.
	child := &processedBlock{
		Block:  b,
		Parent: pb.Block.ID(),

		Height: pb.Height + 1,
		Depth:  pb.childDepth(),
	}
	child.setChildTarget(db)

	// Add the child to the parent.
	pb.Children = append(pb.Children, child.Block.ID())

	db.updateBlockMap(pb)

	return child
}
