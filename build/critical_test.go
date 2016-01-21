package build

import (
	"testing"
)

// TestCritical checks that a panic is called in debug mode.
func TestCritical(t *testing.T) {
	k0 := "critical test killstring"
	killstring := "critical test killstring\n"
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
	killstring := "variadic critical test killstring\n"
	defer func() {
		r := recover()
		if r != killstring {
			t.Error("panic did not work:", r, killstring)
		}
	}()
	Critical(k0, k1, k2, k3)
}
