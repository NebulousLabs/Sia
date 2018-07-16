package build

import (
	"testing"
)

// TestCritical checks that a panic is called in debug mode.
func TestCritical(t *testing.T) {
	k0 := "critical test killstring"
	killstring := "Critical error: critical test killstring\nPlease submit a bug report here: https://gitlab.com/NebulousLabs/Sia/issues\n"
	defer func() {
		r := recover()
		if r != killstring {
			t.Error("panic did not work:", r, killstring)
		}
	}()
	Critical(k0)
}

// TestCriticalVariadic checks that a panic is called in debug mode.
func TestCriticalVariadic(t *testing.T) {
	k0 := "variadic"
	k1 := "critical"
	k2 := "test"
	k3 := "killstring"
	killstring := "Critical error: variadic critical test killstring\nPlease submit a bug report here: https://gitlab.com/NebulousLabs/Sia/issues\n"
	defer func() {
		r := recover()
		if r != killstring {
			t.Error("panic did not work:", r, killstring)
		}
	}()
	Critical(k0, k1, k2, k3)
}

// TestSevere checks that a panic is called in debug mode.
func TestSevere(t *testing.T) {
	k0 := "severe test killstring"
	killstring := "Severe error: severe test killstring\n"
	defer func() {
		r := recover()
		if r != killstring {
			t.Error("panic did not work:", r, killstring)
		}
	}()
	Severe(k0)
}

// TestSevereVariadic checks that a panic is called in debug mode.
func TestSevereVariadic(t *testing.T) {
	k0 := "variadic"
	k1 := "severe"
	k2 := "test"
	k3 := "killstring"
	killstring := "Severe error: variadic severe test killstring\n"
	defer func() {
		r := recover()
		if r != killstring {
			t.Error("panic did not work:", r, killstring)
		}
	}()
	Severe(k0, k1, k2, k3)
}
