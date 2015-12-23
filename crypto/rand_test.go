package crypto

import (
	"testing"
)

// panics returns true if the function fn panicked.
func panics(fn func()) (panicked bool) {
	defer func() {
		panicked = (recover() != nil)
	}()
	fn()
	return
}

// TestRandIntnPanics tests that RandIntn panics if n <= 0.
func TestRandIntnPanics(t *testing.T) {
	// Test n = 0.
	if !panics(func() { RandIntn(0) }) {
		t.Error("expected panic for n <= 0")
	}

	// Test n < 0.
	if !panics(func() { RandIntn(-1) }) {
		t.Error("expected panic for n <= 0")
	}
}
