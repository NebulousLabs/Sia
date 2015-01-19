package consensus

import (
	"math/big"
)

// A BlockNode contains a block and the list of children to the block. Also
// contains some consensus information like which contracts have terminated and
// where there were missed storage proofs.
type BlockNode struct {
	Block    Block
	Children []*BlockNode

	Height           BlockHeight
	Depth            Target        // What the target would need to be to have a weight equal to all blocks up to this block.
	Target           Target        // Target for next block.
	RecentTimestamps [11]Timestamp // The 11 recent timestamps.

	BlockDiff            BlockDiff       // Soon to replace the other 3 entirely
	ContractTerminations []*OpenContract // Contracts that terminated this block.
	MissedStorageProofs  []MissedStorageProof
	SuccessfulWindows    []ContractID
}

// State.childDepth() returns the cumulative weight of all the blocks leading
// up to and including the child block.
// childDepth := (1/parentTarget + 1/parentDepth)^-1
func (bn *BlockNode) childDepth() (depth Target) {
	cumulativeDifficulty := new(big.Rat).Add(bn.Target.Inverse(), bn.Depth.Inverse())
	return RatToTarget(new(big.Rat).Inv(cumulativeDifficulty))
}

// State.childTarget() calculates the proper target of a child node given the
// parent node, and copies the target into the child node.
//
// TODO: stop this function from depending on the state, and also don't pass it
// newNode.
func (s *State) childTarget(parentNode *BlockNode, newNode *BlockNode) Target {
	var timePassed, expectedTimePassed Timestamp
	if newNode.Height < TargetWindow {
		timePassed = newNode.Block.Timestamp - s.blockRoot.Block.Timestamp
		expectedTimePassed = BlockFrequency * Timestamp(newNode.Height)
	} else {
		// TODO: this code make unsafe assumptions - that the block node is on
		// the current fork.
		adjustmentBlock, err := s.BlockAtHeight(newNode.Height - TargetWindow)
		if err != nil {
			panic(err)
		}
		timePassed = newNode.Block.Timestamp - adjustmentBlock.Timestamp
		expectedTimePassed = BlockFrequency * Timestamp(TargetWindow)
	}

	// Adjustment = timePassed / expectedTimePassed.
	targetAdjustment := big.NewRat(int64(timePassed), int64(expectedTimePassed))

	// Enforce a maximum targetAdjustment
	if targetAdjustment.Cmp(MaxAdjustmentUp) == 1 {
		targetAdjustment = MaxAdjustmentUp
	} else if targetAdjustment.Cmp(MaxAdjustmentDown) == -1 {
		targetAdjustment = MaxAdjustmentDown
	}

	newTarget := new(big.Rat).Mul(parentNode.Target.Rat(), targetAdjustment)
	return RatToTarget(newTarget)
}
