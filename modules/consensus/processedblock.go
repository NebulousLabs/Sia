package consensus

import (
	"math/big"
	"sort"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// SurpassThreshold is a percentage that dictates how much heavier a competing
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
	Block       types.Block
	Height      types.BlockHeight
	Depth       types.Target
	ChildTarget types.Target

	DiffsGenerated            bool
	SiacoinOutputDiffs        []modules.SiacoinOutputDiff
	FileContractDiffs         []modules.FileContractDiff
	SiafundOutputDiffs        []modules.SiafundOutputDiff
	DelayedSiacoinOutputDiffs []modules.DelayedSiacoinOutputDiff
	SiafundPoolDiffs          []modules.SiafundPoolDiff

	ConsensusChecksum crypto.Hash
}

// earliestChildTimestamp returns the earliest timestamp that a child node
// can have while still being valid. See section 'Timestamp Rules' in
// Consensus.md.
//
// To boost performance, earliestChildTimestamp is passed a bucket that it can
// use from inside of a boltdb transaction.
func earliestChildTimestamp(blockMap dbBucket, pb *processedBlock) types.Timestamp {
	// Get the previous MedianTimestampWindow timestamps.
	windowTimes := make(types.TimestampSlice, types.MedianTimestampWindow)
	windowTimes[0] = pb.Block.Timestamp
	parent := pb.Block.ParentID
	for i := uint64(1); i < types.MedianTimestampWindow; i++ {
		// If the genesis block is 'parent', use the genesis block timestamp
		// for all remaining times.
		if parent == (types.BlockID{}) {
			windowTimes[i] = windowTimes[i-1]
			continue
		}

		// Get the next parent's bytes. Because the ordering is specific, the
		// parent does not need to be decoded entirely to get the desired
		// information. This provides a performance boost. The id of the next
		// parent lies at the first 32 bytes, and the timestamp of the block
		// lies at bytes 40-48.
		parentBytes := blockMap.Get(parent[:])
		copy(parent[:], parentBytes[:32])
		windowTimes[i] = types.Timestamp(encoding.DecUint64(parentBytes[40:48]))
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
func (cs *ConsensusSet) targetAdjustmentBase(blockMap *bolt.Bucket, pb *processedBlock) *big.Rat {
	// Grab the block that was generated 'TargetWindow' blocks prior to the
	// parent. If there are not 'TargetWindow' blocks yet, stop at the genesis
	// block.
	var windowSize types.BlockHeight
	parent := pb.Block.ParentID
	current := pb.Block.ID()
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
	timePassed := pb.Block.Timestamp - timestamp
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

// setChildTarget computes the target of a blockNode's child. All children of a node
// have the same target.
func (cs *ConsensusSet) setChildTarget(blockMap *bolt.Bucket, pb *processedBlock) {
	// Fetch the parent block.
	var parent processedBlock
	parentBytes := blockMap.Get(pb.Block.ParentID[:])
	err := encoding.Unmarshal(parentBytes, &parent)
	if build.DEBUG && err != nil {
		panic(err)
	}

	if pb.Height%(types.TargetWindow/2) != 0 {
		pb.ChildTarget = parent.ChildTarget
		return
	}
	adjustment := clampTargetAdjustment(cs.targetAdjustmentBase(blockMap, pb))
	adjustedRatTarget := new(big.Rat).Mul(parent.ChildTarget.Rat(), adjustment)
	pb.ChildTarget = types.RatToTarget(adjustedRatTarget)
}

// newChild creates a blockNode from a block and adds it to the parent's set of
// children. The new node is also returned. It necessairly modifies the database
func (cs *ConsensusSet) newChild(tx *bolt.Tx, pb *processedBlock, b types.Block) *processedBlock {
	// Create the child node.
	childID := b.ID()
	child := &processedBlock{
		Block:  b,
		Height: pb.Height + 1,
		Depth:  pb.childDepth(),
	}
	blockMap := tx.Bucket(BlockMap)
	cs.setChildTarget(blockMap, child)
	err := blockMap.Put(childID[:], encoding.Marshal(*child))
	if build.DEBUG && err != nil {
		panic(err)
	}
	return child
}
