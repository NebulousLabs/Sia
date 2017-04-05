package types

import (
	"sort"
	"testing"
)

// TestUnlockHashSliceSorting checks that the sort method correctly sorts
// unlock hash slices.
func TestUnlockHashSliceSorting(t *testing.T) {
	// To test that byte-order is done correctly, a semi-random second byte is
	// used that is equal to the first byte * 23 % 7
	uhs := UnlockHashSlice{
		UnlockHash{4, 1},
		UnlockHash{0, 0},
		UnlockHash{2, 4},
		UnlockHash{3, 6},
		UnlockHash{1, 2},
	}
	sort.Sort(uhs)
	for i := byte(0); i < 5; i++ {
		if uhs[i] != (UnlockHash{i, (i * 23) % 7}) {
			t.Error("sorting failed on index", i, uhs[i])
		}
	}
}
