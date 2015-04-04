package types

import (
	"sort"
	"testing"
)

// TestTimestampSorting verifies that using sort.Sort accurately sorts
// timestamps.
func TestTimestampSorting(t *testing.T) {
	ts := TimestampSlice{
		CurrentTimestamp(),
		CurrentTimestamp() - 5,
		CurrentTimestamp() + 5,
		CurrentTimestamp() + 12,
		CurrentTimestamp() - 3,
		CurrentTimestamp() - 25,
	}

	sort.Sort(ts)
	currentTime := ts[0]
	for _, timestamp := range ts {
		if timestamp < currentTime {
			t.Error("timestamp slice not properly sorted")
		}
		currentTime = timestamp
	}
}
