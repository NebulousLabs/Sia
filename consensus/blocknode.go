package consensus

import (
	"math/big"
)

// a blockNode is an element of a linked list that contains a block and points
// to the block's parent and all of the block's children. It also contains
// context for the block, such as the height, depth, and target of the block,
// which is useful for verifying the block's children. Finally, the blockNode
// contains a set of diffs that explain how the consensus set changes when the
// block is applied or removed. All diffs are fully reversible.
type blockNode struct {
	block    Block
	parent   *blockNode
	children []*blockNode

	height BlockHeight
	depth  Target // Cumulative weight of all parents.
	target Target // Target for next block.

	// Below are the set of diffs for the block, and a bool indicating if the
	// diffs have been generated yet or not. Diffs are computationally
	// expensive to generate, and so will only be generated if at some point
	// the block is a part of the longest fork.
	diffsGenerated        bool
	siafundPoolDiff       SiafundPoolDiff
	siacoinOutputDiffs    []SiacoinOutputDiff
	fileContractDiffs     []FileContractDiff
	siafundOutputDiffs    []SiafundOutputDiff
	delayedSiacoinOutputs map[SiacoinOutputID]SiacoinOutput
}

// childDepth returns the depth that any child node would have.
// childDepth := (1/parentTarget + 1/parentDepth)^-1
func (bn *blockNode) childDepth() (depth Target) {
	cumulativeDifficulty := new(big.Rat).Add(bn.target.Inverse(), bn.depth.Inverse())
	return RatToTarget(new(big.Rat).Inv(cumulativeDifficulty))
}

// setTarget calculates the target for a node and sets the node's target equal
// to the calculated value.
func (node *blockNode) setTarget() {
	// Sanity check - the node should have a parent.
	if DEBUG {
		if node.parent == nil {
			panic("calling setTarget on node with no parent")
		}
	}

	// To calculate the target, we need to compare our timestamp with the
	// timestamp of the reference node, which is `TargetWindow` blocks earlier,
	// or if the height is less than `TargetWindow`, it's the genesis block.
	//
	// There's no easy way to look up the node that is the 'TargetWidow'th
	// parent of the input node, because we're not sure which fork the parent
	// is in, it may not be the current fork. This is not a huge performance
	// concern, becuase 'TargetWindow' is small and blocks are infrequent.
	// Signature verification of the transactions will still be the bottleneck
	// for large blocks.
	//
	// CONTRIBUTE: find a way to look up the correct parent without scrolling
	// through 'TargetWindow' elements in a linked list.
	var i BlockHeight
	referenceNode := node
	for i = 0; i < TargetWindow && referenceNode.parent != nil; i++ {
		referenceNode = referenceNode.parent
	}

	// Calculate the amount to adjust the target by dividing the amount of time
	// passed by the expected amount of time passed.
	timePassed := node.block.Timestamp - referenceNode.block.Timestamp
	expectedTimePassed := BlockFrequency * Timestamp(i)
	targetAdjustment := big.NewRat(int64(timePassed), int64(expectedTimePassed))

	// Enforce a maximum target adjustment.
	if targetAdjustment.Cmp(MaxAdjustmentUp) == 1 {
		targetAdjustment = MaxAdjustmentUp
	} else if targetAdjustment.Cmp(MaxAdjustmentDown) == -1 {
		targetAdjustment = MaxAdjustmentDown
	}

	// Multiply the previous target by the adjustment to get the new target.
	parentTarget := node.parent.target
	newRatTarget := new(big.Rat).Mul(parentTarget.Rat(), targetAdjustment)
	node.target = RatToTarget(newRatTarget)
}

// addBlockToTree takes a block and a parent node, and adds a child node to the
// parent containing the block. No validation is done.
func (s *State) addBlockToTree(b Block) (err error) {
	parentNode := s.blockMap[b.ParentID]
	newNode := &blockNode{
		block:  b,
		parent: parentNode,

		height: parentNode.height + 1,
		depth:  parentNode.childDepth(),

		delayedSiacoinOutputs: make(map[SiacoinOutputID]SiacoinOutput),
	}
	newNode.setTarget()

	// Add the node to the block map and update the list of its parents
	// children.
	s.blockMap[b.ID()] = newNode
	parentNode.children = append(parentNode.children, newNode)

	if s.heavierFork(newNode) {
		err = s.forkBlockchain(newNode)
		if err != nil {
			return
		}
	}

	return
}
