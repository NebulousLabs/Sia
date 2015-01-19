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
