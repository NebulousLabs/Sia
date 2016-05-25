package main

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/types"
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
			time.Date(2016, time.May, 11, 19, 33, 0, 0, time.UTC),
			5e4,
		},
		// 4 minutes later
		{
			time.Date(2016, time.May, 11, 19, 37, 0, 0, time.UTC),
			5e4,
		},
		// 5 minutes later
		{
			time.Date(2016, time.May, 11, 19, 38, 0, 0, time.UTC),
			5e4 + 1,
		},
		// 10 minutes later
		{
			time.Date(2016, time.May, 11, 19, 43, 0, 0, time.UTC),
			5e4 + 1,
		},
		// 1 day later
		{
			time.Date(2016, time.May, 12, 19, 33, 0, 0, time.UTC),
			5e4 + 144,
		},
	}
	for _, tt := range tests {
		h := estimatedHeightAt(tt.t)
		if h != tt.expectedHeight {
			t.Errorf("expected an estimated height of %v, but got %v", tt.expectedHeight, h)
		}
	}
}
