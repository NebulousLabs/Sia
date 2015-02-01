package consensus

import (
	"fmt"
	"math/big"
)

// A BlockNode contains a block and the list of children to the block. Also
// contains some consensus information like which contracts have terminated and
// where there were missed storage proofs.
type blockNode struct {
	block    Block
	parent   *blockNode
	children []*blockNode

	height BlockHeight
	depth  Target // Cumulative weight of all parents.
	target Target // Target for next block.

	diffsGenerated bool
	outputDiffs    []OutputDiff
	contractDiffs  []ContractDiff
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
	// To calculate the target, we need to compare our timestamp with the
	// timestamp of the reference node, which is `TargetWindow` blocks earlier,
	// or if the height is less than `TargetWindow`, it's the genesis block.
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

	// Enforce a maximum targetAdjustment
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

	// Reconnect all orphans that now have a parent.
	childMap := s.missingParents[b.ID()]
	for _, child := range childMap {
		// This is a recursive call, which means someone could make it take
		// a long time by giving us a bunch of false orphan blocks with the
		// same parent. We have to check them all anyway though, it just
		// allows them to cluster the processing. Utimately I think
		// bandwidth will be the bigger issue, we'll reject bad orphans
		// before we verify signatures.
		err = s.validHeader(child)
		if err != nil {
			// TODO: There's nothing we can really do about the error that gets
			// returned, because it doesn't reflect a problem with the current
			// block. It means that someone managed to give us orphans with
			// errors.
			fmt.Println(err)
			continue
		}
		err = s.addBlockToTree(child)
		if err != nil {
			// TODO: There's nothing we can really do about the error that gets
			// returned, because it doesn't reflect a problem with the current
			// block. It means that someone managed to give us orphans with
			// errors.
			fmt.Println(err)
		}

	}
	delete(s.missingParents, b.ID())

	return
}
