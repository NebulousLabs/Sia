package build

import (
	"testing"
)

// TestCritical checks that a panic is called in debug mode.
func TestCritical(t *testing.T) {
	killstring := "critical test killstring\n"
	defer func() {
		r := recover()
		if r != killstring {
			t.Error("panic did not work:", r, killstring)
		}
	}()
	Critical(killstring)
}
