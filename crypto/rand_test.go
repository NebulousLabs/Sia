package crypto

import (
	"testing"
)

// TestRandIntnPanics tests that RandIntn panics if n <= 0.
func TestRandIntnPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for n <= 0")
		}
	}()
	RandIntn(0)
	RandIntn(-1)
}
