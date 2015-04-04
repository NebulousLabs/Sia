package types

import (
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// TestCheckBuildConstants checks that the required build constants have been
// set.
func TestCheckBuildConstants(t *testing.T) {
	if !build.DEBUG {
		t.Error("DEBUG needs to be enabled for testing to work.")
		t.Error(build.DEBUG)
	}
	if build.Release != "testing" {
		t.Error("build.Release needs to be set to \"testing\"")
		t.Error(build.Release)
	}
}
