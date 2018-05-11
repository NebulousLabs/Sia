package consensus

import (
	"sort"

	"github.com/NebulousLabs/Sia/modules/consensus/database"
	"github.com/NebulousLabs/Sia/types"
)

// blockRuleHelper assists with block validity checks by calculating values
// on blocks that are relevant to validity rules.
type blockRuleHelper interface {
	minimumValidChildTimestamp(database.Tx, *database.Block) types.Timestamp
}

// stdBlockRuleHelper is the standard implementation of blockRuleHelper.
type stdBlockRuleHelper struct{}

// minimumValidChildTimestamp returns the earliest timestamp that a child node
// can have while still being valid. See section 'Block Timestamps' in
// Consensus.md.
//
// To boost performance, minimumValidChildTimestamp is passed a bucket that it
// can use from inside of a boltdb transaction.
func (rh stdBlockRuleHelper) minimumValidChildTimestamp(tx database.Tx, b *database.Block) types.Timestamp {
	// Get the previous MedianTimestampWindow timestamps.
	windowTimes := make(types.TimestampSlice, types.MedianTimestampWindow)
	windowTimes[0] = b.Timestamp
	parent := b.ParentID
	for i := uint64(1); i < types.MedianTimestampWindow; i++ {
		// If the genesis block is 'parent', use the genesis block timestamp
		// for all remaining times.
		if parent == (types.BlockID{}) {
			windowTimes[i] = windowTimes[i-1]
			continue
		}

		// Get the next parent ID and timestamp
		parentBlock, _ := tx.Block(parent)
		parent = parentBlock.ParentID
		windowTimes[i] = parentBlock.Timestamp
	}
	sort.Sort(windowTimes)

	// Return the median of the sorted timestamps.
	return windowTimes[len(windowTimes)/2]
}
