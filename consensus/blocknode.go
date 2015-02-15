package consensus

import (
	"math/big"
	"sort"
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
	block    Block
	parent   *blockNode
	children []*blockNode

	height BlockHeight
	depth  Target // Cumulative weight of all parents.
	target Target // Target for next block, i.e. any child blockNodes.

	// Diffs are computationally expensive to generate, so a lazy approach is
	// taken wherein the diffs are only generated when needed. A boolean
	// prevents duplicate work from being performed.
	diffsGenerated        bool
	siafundPoolDiff       SiafundPoolDiff
	siacoinOutputDiffs    []SiacoinOutputDiff
	fileContractDiffs     []FileContractDiff
	siafundOutputDiffs    []SiafundOutputDiff
	delayedSiacoinOutputs map[SiacoinOutputID]SiacoinOutput
}

// childDepth returns the depth of a blockNode's child nodes. The depth is the
// "sum" of the current depth and current target, where sum is defined as:
//
//     sum(x,y) := 1/(1/x + 1/y)
func (bn *blockNode) childDepth() (depth Target) {
	cumulativeDifficulty := new(big.Rat).Add(bn.target.Inverse(), bn.depth.Inverse())
	return RatToTarget(new(big.Rat).Inv(cumulativeDifficulty))
}

// heavierThan returns true if the blockNode is sufficiently heavier than
// 'cmp', where "sufficient" is defined as:
//
//     (1/bn.depth) - (1/cmp.depth) > (1/cmp.target) * SurpassThreshold
func (bn *blockNode) heavierThan(cmp *blockNode) bool {
	diff := new(big.Rat).Sub(bn.depth.Inverse(), cmp.depth.Inverse())
	threshold := new(big.Rat).Mul(cmp.target.Inverse(), SurpassThreshold)
	return diff.Cmp(threshold) > 0
}

// earliestChildTimestamp returns the earliest timestamp that a child node
// can have while still being valid. See section 'Timestamp Rules' in
// Consensus.md.
func (bn *blockNode) earliestChildTimestamp() Timestamp {
	// Get the previous `MedianTimestampWindow` timestamps.
	var intTimestamps []int
	referenceNode := bn
	for i := 0; i < MedianTimestampWindow; i++ {
		intTimestamps = append(intTimestamps, int(referenceNode.block.Timestamp))
		if referenceNode.parent != nil {
			referenceNode = referenceNode.parent
		}
	}
	sort.Ints(intTimestamps)

	// Return the median of the sorted timestamps.
	return Timestamp(intTimestamps[MedianTimestampWindow/2])
}

// newChild creates a blockNode from a block and adds it to the parent's set of
// children. The new node is also returned.
func (bn *blockNode) newChild(b Block) *blockNode {
	// Sanity check - parent can't be nil.
	if DEBUG {
		if bn == nil {
			panic("can't create blockNode with nil parent")
		}
	}

	child := &blockNode{
		block:  b,
		parent: bn,

		height: bn.height + 1,
		depth:  bn.childDepth(),

		delayedSiacoinOutputs: make(map[SiacoinOutputID]SiacoinOutput),
	}

	// Calculate the target for the new node. To calculate the target, we need
	// to compare our timestamp with the timestamp of the reference node, which
	// is `TargetWindow` blocks earlier, or if the height is less than
	// `TargetWindow`, it's the genesis block.
	//
	// CONTRIBUTE: find a way to look up the correct parent without scrolling
	// through 'TargetWindow' elements in a linked list.
	var numBlocks BlockHeight
	windowStart := child
	for numBlocks = 0; numBlocks < TargetWindow && windowStart.parent != nil; numBlocks++ {
		windowStart = windowStart.parent
	}

	// Calculate the amount to adjust the target by dividing the amount of time
	// passed by the expected amount of time passed.
	timePassed := child.block.Timestamp - windowStart.block.Timestamp
	expectedTimePassed := BlockFrequency * numBlocks
	targetAdjustment := big.NewRat(int64(timePassed), int64(expectedTimePassed))

	// Clamp adjustment to reasonable values.
	if targetAdjustment.Cmp(MaxAdjustmentUp) > 0 {
		targetAdjustment = MaxAdjustmentUp
	} else if targetAdjustment.Cmp(MaxAdjustmentDown) < 0 {
		targetAdjustment = MaxAdjustmentDown
	}

	// Multiply the previous target by the adjustment to get the new target.
	newRatTarget := new(big.Rat).Mul(bn.target.Rat(), targetAdjustment)
	child.target = RatToTarget(newRatTarget)

	// add child to parent
	bn.children = append(bn.children, child)

	return child
}
