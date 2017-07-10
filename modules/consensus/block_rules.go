package consensus

import (
	"sort"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// blockRuleHelper assists with block validity checks by calculating values
// on blocks that are relevant to validity rules.
type blockRuleHelper interface {
	minimumValidChildTimestamp(blockMap dbBucket, parentID types.BlockID, blockTimestamp types.Timestamp) types.Timestamp
}

// stdBlockRuleHelper is the standard implementation of blockRuleHelper.
type stdBlockRuleHelper struct{}

// minimumValidChildTimestamp returns the earliest timestamp that a child node
// can have while still being valid. See section 'Block Timestamps' in
// Consensus.md.
//
// To boost performance, minimumValidChildTimestamp is passed a bucket that it
// can use from inside of a boltdb transaction.
func (rh stdBlockRuleHelper) minimumValidChildTimestamp(blockMap dbBucket, parentID types.BlockID, blockTimestamp types.Timestamp) types.Timestamp {
	// Get the previous MedianTimestampWindow timestamps.
	windowTimes := make(types.TimestampSlice, types.MedianTimestampWindow)
	windowTimes[0] = blockTimestamp
	for i := uint64(1); i < types.MedianTimestampWindow; i++ {
		// If the genesis block is 'parent', use the genesis block timestamp
		// for all remaining times.
		if parentID == (types.BlockID{}) {
			windowTimes[i] = windowTimes[i-1]
			continue
		}

		// Get the next parent's bytes. Because the ordering is specific, the
		// parent does not need to be decoded entirely to get the desired
		// information. This provides a performance boost. The id of the next
		// parent lies at the first 32 bytes, and the timestamp of the block
		// lies at bytes 40-48.
		parentBytes := blockMap.Get(parentID[:])
		copy(parentID[:], parentBytes[:32])
		windowTimes[i] = types.Timestamp(encoding.DecUint64(parentBytes[40:48]))
	}
	sort.Sort(windowTimes)

	// Return the median of the sorted timestamps.
	return windowTimes[len(windowTimes)/2]
}
