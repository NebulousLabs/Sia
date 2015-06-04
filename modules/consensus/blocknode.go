package consensus

import (
	"math/big"
	"sort"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// SurpassThreshold is a percentage that dictates how much heavier a competing
// chain has to be before the node will switch to mining on that chain. This is
// not a consensus rule. This percentage is only applied to the most recent
// block, not the entire chain; see blockNode.heavierThan.
//
// If no threshold were in place, it would be possible to manipulate a block's
// timestamp to produce a sufficiently heavier block.
var SurpassThreshold = big.NewRat(20, 100)

// a blockNode is a node in the tree of competing blockchain forks. It contains
// the block itself, parent and child blockNodes, and context such as the
// block height, depth, and target. It also contains a set of diffs that
// dictate how the consensus set is affected by the block.
type blockNode struct {
	block    types.Block
	parent   *blockNode
	children []*blockNode

	height      types.BlockHeight
	depth       types.Target // Cumulative weight of all parents.
	childTarget types.Target // Target for next block, i.e. any child blockNodes.

	// Diffs are computationally expensive to generate, so a lazy approach is
	// taken wherein the diffs are only generated when needed. A boolean
	// prevents duplicate work from being performed.
	//
	// Note that diffsGenerated == true iff the node has ever been in the
	// State's currentPath; this is because diffs must be generated to apply
	// the node.
	diffsGenerated            bool
	siacoinOutputDiffs        []modules.SiacoinOutputDiff
	fileContractDiffs         []modules.FileContractDiff
	siafundOutputDiffs        []modules.SiafundOutputDiff
	delayedSiacoinOutputDiffs []modules.DelayedSiacoinOutputDiff
	siafundPoolDiff           modules.SiafundPoolDiff
}

// earliestChildTimestamp returns the earliest timestamp that a child node
// can have while still being valid. See section 'Timestamp Rules' in
// Consensus.md.
func (bn *blockNode) earliestChildTimestamp() types.Timestamp {
	// Get the previous MedianTimestampWindow timestamps.
	windowTimes := make(types.TimestampSlice, types.MedianTimestampWindow)
	current := bn
	for i := 0; i < types.MedianTimestampWindow; i++ {
		windowTimes[i] = current.block.Timestamp

		// If we are at the genesis block, keep using the genesis block for the
		// remaining times.
		if current.parent != nil {
			current = current.parent
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
func (bn *blockNode) heavierThan(cmp *blockNode) bool {
	requirement := cmp.depth.AddDifficulties(cmp.childTarget.MulDifficulty(SurpassThreshold))
	return requirement.Cmp(bn.depth) > 0 // Inversed, because the smaller target is actually heavier.
}

// childDepth returns the depth of a blockNode's child nodes. The depth is the
// "sum" of the current depth and current difficulty. See target.Add for more
// detailed information.
func (bn *blockNode) childDepth() types.Target {
	return bn.depth.AddDifficulties(bn.childTarget)
}

// targetAdjustmentBase returns the magnitude that the target should be
// adjusted by before a clamp is applied.
func (bn *blockNode) targetAdjustmentBase() *big.Rat {
	// Target only adjusts twice per window.
	if bn.height%(types.TargetWindow/2) != 0 {
		return big.NewRat(1, 1)
	}

	// Grab the block that was generated 'TargetWindow' blocks prior to the
	// parent. If there are not 'TargetWindow' blocks yet, stop at the genesis
	// block.
	var windowSize types.BlockHeight
	windowStart := bn
	for windowSize = 0; windowSize < types.TargetWindow && windowStart.parent != nil; windowSize++ {
		windowStart = windowStart.parent
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
	timePassed := bn.block.Timestamp - windowStart.block.Timestamp
	expectedTimePassed := types.BlockFrequency * windowSize
	return big.NewRat(int64(timePassed), int64(expectedTimePassed))
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

// setTarget computes the target of a blockNode's child. All children of a node
// have the same target.
func (bn *blockNode) setChildTarget() {
	adjustment := clampTargetAdjustment(bn.targetAdjustmentBase())
	adjustedRatTarget := new(big.Rat).Mul(bn.parent.childTarget.Rat(), adjustment)
	bn.childTarget = types.RatToTarget(adjustedRatTarget)
}

// newChild creates a blockNode from a block and adds it to the parent's set of
// children. The new node is also returned.
func (bn *blockNode) newChild(b types.Block) *blockNode {
	// Create the child node.
	child := &blockNode{
		block:  b,
		parent: bn,

		height: bn.height + 1,
		depth:  bn.childDepth(),
	}
	child.setChildTarget()

	// Add the child to the parent.
	bn.children = append(bn.children, child)

	return child
}
