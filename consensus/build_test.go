package consensus

import (
	"testing"
)

// TestConstants makes sure that the testing constants are being used instead
// of the developer constants or the release constants.
func TestConstants(t *testing.T) {
	if RootTarget[0] != 64 {
		panic("using wrong constant during testing!")
	}
	if !DEBUG {
		panic("using wrong constant during testing, DEBUG flag needs to be set")
	}
}
