package consensus

import (
	"bytes"
	"encoding/binary"
	"math/big"

	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
	"github.com/NebulousLabs/errors"
)

var (
	// errOakHardforkIncompatibility is the error returned if Oak initialization
	// cannot begin because the consensus database was not upgraded before the
	// hardfork height.
	errOakHardforkIncompatibility = errors.New("difficulty adjustment hardfork incompatibility detected")
)

// difficulty.go defines the Oak difficulty adjustment algorithm. Past the
// hardfork trigger height, it the algorithm that Sia uses to adjust the
// difficulty.
//
// A running tally is maintained which keeps the total difficulty and total time
// passed across all blocks. The total difficulty can be divided by the total
// time to get a hashrate. The total is multiplied by 0.995 each block, to keep
// exponential preference on recent blocks with a half life of about 24 hours.
// This estimated hashrate is assumed to closely match the actual hashrate on
// the network.
//
// There is a target block time. If the difficulty increases or decreases, the
// total amount of time that has passed will be more or less than the target
// amount of time passed for the current height. To counteract this, the target
// block time for each block is adjusted based on how far away from the desired
// total time passed the current total time passed is. If the total time passed
// is too low, blocks are targeted to be slightly longer, which helps to correct
// the network. And if the total time passed is too high, blocks are targeted to
// be slightly shorter, to help correct the network.
//
// High variance in block times means that the corrective action should not be
// very strong if the total time passed has only missed the target time passed
// by a few hours. But if the total time passed is significantly off, the block
// time corrections should be much stronger. The square of the total deviation
// is used to figure out what the adjustment should be. At 10,000 seconds
// variance (about 3 hours), blocks will be adjusted by 10 seconds each. At
// 20,000 seconds, blocks will be adjusted by 40 seconds each, a 4x adjustment
// for 2x the error. And at 40,000 seconds, blocks will be adjusted by 160
// seconds each, and so on.
//
// The total amount of blocktime adjustment is capped to 1/3 and 3x the target
// blocktime, to prevent too much disruption on the network. If blocks are
// actually coming out 3x as fast as intended, there will be a (temporary)
// significant increase on the amount of strain on nodes to process blocks. And
// at 1/3 the target blocktime, the total blockchain throughput will decrease
// dramatically.
//
// Finally, one extra cap is applied to the difficulty adjustment - the
// difficulty of finding a block is not allowed to change more than 0.4% every
// block. This maps to a total possible difficulty change of 55x across 1008
// blocks. This clamp helps to prevent wild swings when the hashrate increases
// or decreases rapidly on the network, and it also limits the amount of damange
// that a malicious attacker can do if performing a difficulty raising attack.

// childTargetOak sets the child target based on the total time delta and total
// hashrate of the parent block. The deltas are known for the child block,
// however we do not use the child block deltas because that would allow the
// child block to influence the target of the following block, which makes abuse
// easier in selfish mining scenarios.
func (cs *ConsensusSet) childTargetOak(parentTotalTime int64, parentTotalTarget, parentTarget types.Target, parentHeight types.BlockHeight) types.Target {
	// Determine the detla of the current total time vs. the desired total time.
	expectedTime := types.BlockFrequency * parentHeight
	delta := int64(expectedTime) - parentTotalTime
	square := delta * delta
	if delta < 0 {
		// If the delta is negative, restore the negative value.
		square *= -1
	}

	// Convert the delta in to a target block time. Clamp the target block time
	// to 1/3 and 3x the real block time.
	delta /= 10e6 // 10e3 second delta leads to 10 second shift.
	targetBlockTime := int64(types.BlockFrequency) + delta
	if targetBlockTime < int64(types.BlockFrequency)/3 {
		targetBlockTime = int64(types.BlockFrequency) / 3
	}
	if targetBlockTime > int64(types.BlockFrequency)*3 {
		targetBlockTime = int64(types.BlockFrequency) * 3
	}

	// Determine the hashrate using the total time and total target. Set a
	// minimum total time of 1 to prevent divide by zero and underflows.
	if parentTotalTime < 1 {
		parentTotalTime = 1
	}
	visibleHashrate := parentTotalTarget.Difficulty().Div64(uint64(parentTotalTime)) // Hashes per second.

	// Determine the new target by multiplying the visible hashrate by the
	// target block time. Clamp it to a 0.4% difficulty adjustment.
	maxNewTarget := parentTarget.MulDifficulty(types.OakDifficultyMaxRise)
	minNewTarget := parentTarget.MulDifficulty(types.OakDifficultyMaxDrop)
	newTarget := types.RatToTarget(new(big.Rat).SetFrac(types.RootDepth.Int(), visibleHashrate.Mul64(uint64(targetBlockTime)).Big()))
	if newTarget.Cmp(maxNewTarget) < 0 {
		newTarget = maxNewTarget
	}
	if newTarget.Cmp(minNewTarget) > 0 {
		newTarget = minNewTarget
	}
	return newTarget
}

// storeBlockTotals computes the new total time and total target for the current
// block and stores that new time in the database. It also returns the new
// totals.
func (cs *ConsensusSet) storeBlockTotals(tx *bolt.Tx, currentHeight types.BlockHeight, currentBlockID types.BlockID, prevTotalTime int64, parentTimestamp, currentTimestamp types.Timestamp, prevTotalTarget, targetOfCurrentBlock types.Target) (newTotalTime int64, newTotalTarget types.Target, err error) {
	// Reset the prevTotalTime to a delta of zero just before the hardfork.
	if currentHeight == types.OakHardforkBlock-1 {
		prevTotalTime = int64(types.BlockFrequency * currentHeight)
	}

	// For each value, first multiply by the decay, and then add in the new
	// delta.
	newTotalTime = (prevTotalTime * types.OakDifficultyDecayNum / types.OakDifficultyDecayDenom) + (int64(currentTimestamp) - int64(parentTimestamp))
	newTotalTarget = prevTotalTarget.MulDifficulty(big.NewRat(types.OakDifficultyDecayNum, types.OakDifficultyDecayDenom)).AddDifficulties(targetOfCurrentBlock)

	// Store the new total time and total target in the database at the
	// appropriate id.
	bytes := make([]byte, 72, 72)
	binary.LittleEndian.PutUint64(bytes[:8], uint64(newTotalTime))
	copy(bytes[8:40], newTotalTarget[:])
	copy(bytes[40:], targetOfCurrentBlock[:])
	err = tx.Bucket(BucketOak).Put(currentBlockID[:], bytes)
	if err != nil {
		return 0, types.Target{}, errors.Extend(errors.New("unable to store total time values"), err)
	}
	return newTotalTime, newTotalTarget, nil
}

// initOak will initialize all of the oak difficulty adjustment related fields.
// This is separate from the initialization process for compatibility reasons -
// some databases will not have these fields at start, so it much be checked.
//
// After oak initialization is complete, a specific field in the oak bucket is
// marked so that oak initialization can be skipped in the future.
func (cs *ConsensusSet) initOak(tx *bolt.Tx) error {
	// Prep the oak bucket.
	bucketOak, err := tx.CreateBucketIfNotExists(BucketOak)
	if err != nil {
		return errors.Extend(errors.New("unable to create oak bucket"), err)
	}
	// Check whether the init field is set.
	if bytes.Equal(bucketOak.Get(FieldOakInit), ValueOakInit) {
		// The oak fields have been initialized, nothing to do.
		return nil
	}

	// If the current height is greater than the hardfork trigger date, return
	// an error and refuse to initialize.
	height := blockHeight(tx)
	if height > types.OakHardforkBlock {
		return errOakHardforkIncompatibility
	}

	// Store base values for the genesis block.
	_, _, err = cs.storeBlockTotals(tx, 0, types.GenesisID, 0, types.GenesisTimestamp, types.GenesisTimestamp, types.RootDepth, types.RootTarget)
	if err != nil {
		return errors.Extend(errors.New("unable to store genesis block totals"), err)
	}

	// The Oak fields have not been initialized, scan through the consensus set
	// and set the fields for each block.
	var totalTime int64
	var totalTarget types.Target
	parentTimestamp := types.GenesisTimestamp
	parentChildTarget := types.RootTarget
	for i := types.BlockHeight(1); i <= height; i++ { // Skip Genesis block
		// Fetch the processed block for the current block.
		id, err := getPath(tx, i)
		if err != nil {
			return errors.Extend(errors.New("unable to find block at height"), err)
		}
		pb, err := getBlockMap(tx, id)
		if err != nil {
			return errors.Extend(errors.New("unable to find block from id"), err)
		}

		// Calculate and store the new block totals.
		totalTime, totalTarget, err = cs.storeBlockTotals(tx, i, id, totalTime, parentTimestamp, pb.Block.Timestamp, totalTarget, parentChildTarget)
		if err != nil {
			return errors.Extend(errors.New("unable to store updated block totals"), err)
		}
		// Update the previous values.
		parentTimestamp = pb.Block.Timestamp
		parentChildTarget = pb.ChildTarget
	}

	// Tag the initialization field in the oak bucket, indicating that
	// initialization has completed.
	err = bucketOak.Put(FieldOakInit, ValueOakInit)
	if err != nil {
		return errors.Extend(errors.New("unable to put oak init confirmation into oak bucket"), err)
	}
	return nil
}
