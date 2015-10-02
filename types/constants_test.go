package types

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// TestCheckBuildConstants checks that the required build constants have been
// set.
func TestCheckBuildConstants(t *testing.T) {
	// Verify that the build has been set to 'testing'.
	if build.Release != "testing" {
		t.Error("build.Release needs to be set to \"testing\"")
		t.Error(build.Release)
	}
	if testing.Short() {
		t.SkipNow()
	}
	// Verify that, for the longer tests, the 'debug' build tag has been used.
	if !build.DEBUG {
		t.Error("DEBUG needs to be enabled for testing to work.")
		t.Error(build.DEBUG)
	}
}
