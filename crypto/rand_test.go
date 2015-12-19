package crypto

import (
	"testing"
)

// TestRandIntnPanics tests that RandIntn panics if n <= 0. The crypto/rand
// package guarantees that rand.Int will panic if n <= 0, but other random
// packages might not.
func TestRandIntnPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for n <= 0")
		}
	}()
	RandIntn(0)
	RandIntn(-1)
}
