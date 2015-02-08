package consensus

import (
	"bytes"
	"math/big"

	"github.com/NebulousLabs/Sia/crypto"
)

// A non-consensus rule that dictates how much heavier a competing chain has to
// be before the node will switch to mining on that chain. The percent refers
// to the percent of the weight of the most recent block on the winning chain,
// not the weight of the entire chain.
//
// This rule is in place because the target gets updated every block, and that
// means that of two competing blocks, one could be very slightly heavier. The
// slightly heavier one should not be switched to if it was not seen first,
// because the amount of extra weight in the chain is inconsequential. The
// maximum difficulty shift will prevent people from manipulating timestamps
// enough to produce a block that is substantially heavier.
var (
	SurpassThreshold = big.NewRat(20, 100)
)

// a blockNode is an element of a linked list that contains a block and points
// to the block's parent and all of the block's children. It also contains
// context for the block, such as the height, depth, and target of the block,
// which is useful for verifying the block's children. Finally, the blockNode
// contains a set of diffs that explain how the consensus set changes when the
// block is applied or removed. All diffs are fully reversible.
//
// Each block has a target, which is the target that all child blocks need to
// meet. A target is considered 'met' if the numerical representation of the
// block id is less than the numerical representation of the target. A useful
// property of the target is that the 'difficulty', or expected amount of work,
// can be determined by taking the inverse of the target. Furthermore, two
// difficulties can be added together, and the inverse of the sum produces a
// new target that would be equally as difficult as finding each of the
// original targets once.
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

// CheckTarget returns true if the block id is lower than the target.
func (b Block) CheckTarget(target Target) bool {
	blockHash := b.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// Int returns a Target as a big.Int.
func (t Target) Int() *big.Int {
	return new(big.Int).SetBytes(t[:])
}

// Rat returns a Target as a big.Rat.
func (t Target) Rat() *big.Rat {
	return new(big.Rat).SetInt(t.Int())
}

// Inv returns the inverse of a Target as a big.Rat
func (t Target) Inverse() *big.Rat {
	r := t.Rat()
	return r.Inv(r)
}

// IntToTarget converts a big.Int to a Target.
func IntToTarget(i *big.Int) (t Target) {
	// i may overflow the maximum target.
	// In the event of overflow, return the maximum.
	if i.BitLen() > 256 {
		return RootDepth
	}
	b := i.Bytes()
	// need to preserve big-endianness
	offset := crypto.HashSize - len(b)
	copy(t[offset:], b)
	return
}

// RatToTarget converts a big.Rat to a Target.
func RatToTarget(r *big.Rat) Target {
	// convert to big.Int to truncate decimal
	i := new(big.Int).Div(r.Num(), r.Denom())
	return IntToTarget(i)
}

// childDepth returns the depth that any child node would have.
//
// childDepth = (1/parentTarget + 1/parentDepth)^-1
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
	// concern, because 'TargetWindow' is small and blocks are infrequent.
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

// heavierNode compares the depth of `newNode` to the depth of the current
// block node, and returns true if `newNode` is sufficiently heavier, where
// sufficiently is defined by the weight of the current block times
// `SurpassThreshold`.
func (s *State) heavierNode(newNode *blockNode) bool {
	threshold := new(big.Rat).Mul(s.currentBlockWeight(), SurpassThreshold)
	currentCumDiff := s.depth().Inverse()
	requiredCumDiff := new(big.Rat).Add(currentCumDiff, threshold)
	newNodeCumDiff := newNode.depth.Inverse()
	return newNodeCumDiff.Cmp(requiredCumDiff) == 1
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

	if s.heavierNode(newNode) {
		err = s.forkBlockchain(newNode)
		if err != nil {
			return
		}
	}

	return
}
