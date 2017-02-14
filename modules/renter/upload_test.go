package renter

import (
	"testing"
)

// TestRenterSiapathValidate verifies that the validateSiapath function correctly validates SiaPaths.
func TestRenterSiapathValidate(t *testing.T) {
	var pathtests = []struct {
		in    string
		valid bool
	}{
		{"valid/siapath", true},
		{"../../../directory/traversal", false},
		{"testpath", true},
		{"valid/siapath/../with/directory/traversal", false},
		{"validpath/test", true},
		{"..validpath/..test", true},
		{"./invalid/path", false},
		{"test/path", true},
		{"/leading/slash", false},
		{"", false},
	}
	for _, pathtest := range pathtests {
		err := validateSiapath(pathtest.in)
		if err != nil && pathtest.valid {
			t.Fatal("validateSiapath failed on valid path: ", pathtest.in)
		}
		if err == nil && !pathtest.valid {
			t.Fatal("validateSiapath succeeded on invalid path: ", pathtest.in)
		}
	}
}
