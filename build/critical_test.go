package build

import (
	"testing"
)

// TestCritical checks that a panic is called in debug mode.
func TestCritical(t *testing.T) {
	k0 := "critical test killstring"
	killstring := "Critical error: critical test killstring\nPlease submit a bug report here: https://github.com/NebulousLabs/Sia/issues\n"
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
	killstring := "Critical error: variadic critical test killstring\nPlease submit a bug report here: https://github.com/NebulousLabs/Sia/issues\n"
	defer func() {
		r := recover()
		if r != killstring {
			t.Error("panic did not work:", r, killstring)
		}
	}()
	Critical(k0, k1, k2, k3)
}
