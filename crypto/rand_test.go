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

	_, err := RandIntn(0)
	if err != nil {
		t.Error("expected panic on n <= 0, not error")
	}

	_, err = RandIntn(-1)
	if err != nil {
		t.Error("expected panic on n <= 0, not error")
	}
}
