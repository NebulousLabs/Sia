package main

import (
	"testing"
	"time"

	"gitlab.com/NebulousLabs/Sia/types"
)

// TestEstimatedHeightAt tests that the expectedHeightAt function correctly
// estimates the blockheight (and rounds to the nearest block).
func TestEstimatedHeightAt(t *testing.T) {
	tests := []struct {
		t              time.Time
		expectedHeight types.BlockHeight
	}{
		// Test on the same block that is used to estimate the height
		{
			time.Date(2017, time.April, 13, 23, 29, 49, 0, time.UTC),
			100e3,
		},
		// 4 minutes later
		{
			time.Date(2017, time.April, 13, 23, 33, 49, 0, time.UTC),
			100e3,
		},
		// 5 minutes later
		{
			time.Date(2017, time.April, 13, 23, 34, 49, 0, time.UTC),
			100e3 + 1,
		},
		// 15 minutes later
		{
			time.Date(2017, time.April, 13, 23, 44, 49, 0, time.UTC),
			100e3 + 2,
		},
		// 1 day later
		{
			time.Date(2017, time.April, 14, 23, 29, 49, 0, time.UTC),
			100e3 + 160,
		},
	}
	for _, tt := range tests {
		h := estimatedHeightAt(tt.t)
		if h != tt.expectedHeight {
			t.Errorf("expected an estimated height of %v, but got %v", tt.expectedHeight, h)
		}
	}
}
