package consensus

import (
	"math/big"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules/consensus/database"
	"github.com/NebulousLabs/Sia/types"

	"github.com/coreos/bbolt"
)

// SurpassThreshold is a percentage that dictates how much heavier a competing
// chain has to be before the node will switch to mining on that chain. This is
// not a consensus rule. This percentage is only applied to the most recent
// block, not the entire chain; see blockNode.heavierThan.
//
// If no threshold were in place, it would be possible to manipulate a block's
// timestamp to produce a sufficiently heavier block.
var SurpassThreshold = big.NewRat(20, 100)

// heavierThan returns true if the blockNode is sufficiently heavier than
// 'cmp'. 'cmp' is expected to be the current block node. "Sufficient" means
// that the weight of 'b' exceeds the weight of 'cmp' by:
//		(the target of 'cmp' * 'Surpass Threshold')
func heavierThan(b, cmp *database.Block) bool {
	requirement := cmp.Depth.AddDifficulties(cmp.ChildTarget.MulDifficulty(SurpassThreshold))
	return requirement.Cmp(b.Depth) > 0 // Inversed, because the smaller target is actually heavier.
}

// targetAdjustmentBase returns the magnitude that the target should be
// adjusted by before a clamp is applied.
func (cs *ConsensusSet) targetAdjustmentBase(blockMap *bolt.Bucket, b *database.Block) *big.Rat {
	// Grab the block that was generated 'TargetWindow' blocks prior to the
	// parent. If there are not 'TargetWindow' blocks yet, stop at the genesis
	// block.
	var windowSize types.BlockHeight
	parent := b.Block.ParentID
	current := b.Block.ID()
	for windowSize = 0; windowSize < types.TargetWindow && parent != (types.BlockID{}); windowSize++ {
		current = parent
		copy(parent[:], blockMap.Get(parent[:])[:32])
	}
	timestamp := types.Timestamp(encoding.DecUint64(blockMap.Get(current[:])[40:48]))

	// The target of a child is determined by the amount of time that has
	// passed between the generation of its immediate parent and its
	// TargetWindow'th parent. The expected amount of seconds to have passed is
	// TargetWindow*BlockFrequency. The target is adjusted in proportion to how
	// time has passed vs. the expected amount of time to have passed.
	//
	// The target is converted to a big.Rat to provide infinite precision
	// during the calculation. The big.Rat is just the int representation of a
	// target.
	timePassed := b.Block.Timestamp - timestamp
	expectedTimePassed := types.BlockFrequency * windowSize
	return big.NewRat(int64(timePassed), int64(expectedTimePassed))
}

// clampTargetAdjustment returns a clamped version of the base adjustment
// value. The clamp keeps the maximum adjustment to ~7x every 2000 blocks. This
// ensures that raising and lowering the difficulty requires a minimum amount
// of total work, which prevents certain classes of difficulty adjusting
// attacks.
func clampTargetAdjustment(base *big.Rat) *big.Rat {
	if base.Cmp(types.MaxTargetAdjustmentUp) > 0 {
		return types.MaxTargetAdjustmentUp
	} else if base.Cmp(types.MaxTargetAdjustmentDown) < 0 {
		return types.MaxTargetAdjustmentDown
	}
	return base
}

// setChildTarget computes the target of a blockNode's child. All children of a node
// have the same target.
func (cs *ConsensusSet) setChildTarget(blockMap *bolt.Bucket, b *database.Block) {
	// Fetch the parent block.
	var parent database.Block
	parentBytes := blockMap.Get(b.Block.ParentID[:])
	err := encoding.Unmarshal(parentBytes, &parent)
	if build.DEBUG && err != nil {
		panic(err)
	}

	if b.Height%(types.TargetWindow/2) != 0 {
		b.ChildTarget = parent.ChildTarget
		return
	}
	adjustment := clampTargetAdjustment(cs.targetAdjustmentBase(blockMap, b))
	adjustedRatTarget := new(big.Rat).Mul(parent.ChildTarget.Rat(), adjustment)
	b.ChildTarget = types.RatToTarget(adjustedRatTarget)
}

// newChild creates a blockNode from a block and adds it to the parent's set of
// children. The new node is also returned. It necessarily modifies the database
func (cs *ConsensusSet) newChild(tx database.Tx, db *database.Block, b types.Block) *database.Block {
	// Create the child node.
	childID := b.ID()
	child := &database.Block{
		Block:  b,
		Height: db.Height + 1,
		Depth:  db.ChildDepth(),
	}

	// Push the total values for this block into the oak difficulty adjustment
	// bucket. The previous totals are required to compute the new totals.
	prevTotalTime, prevTotalTarget := cs.getBlockTotals(tx, b.ParentID)
	_, _, err := cs.storeBlockTotals(tx, child.Height, childID, prevTotalTime, db.Block.Timestamp, b.Timestamp, prevTotalTarget, db.ChildTarget)
	if build.DEBUG && err != nil {
		panic(err)
	}

	// Use the difficulty adjustment algorithm to set the target of the child
	// block and put the new processed block into the database.
	blockMap := tx.Bucket(BlockMap)
	if db.Height < types.OakHardforkBlock {
		cs.setChildTarget(blockMap, child)
	} else {
		child.ChildTarget = cs.childTargetOak(prevTotalTime, prevTotalTarget, db.ChildTarget, db.Height, db.Block.Timestamp)
	}
	err = blockMap.Put(childID[:], encoding.Marshal(*child))
	if build.DEBUG && err != nil {
		panic(err)
	}
	return child
}
