package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules/tester"
)

// The tests in this file need to be checked manually, even if errors are
// returned it will not be apparent to the test suite.

func TestMain(t *testing.T) {
	// Get a test folder.
	testDir := tester.TempDir("siakg", "TestMain")

	// Create a default set of keys. The result should be 'DefaultTotalKeys'
	// files being created.
	defaultsDir := filepath.Join(testDir, "defaults")
	os.Args = []string{
		"siakg",
		"-f",
		defaultsDir,
	}
	main()

	// Print out the value for one of those keys, the result should be a key
	// that is 'DefaultRequiredKeys' of 'DefaultTotalKeys', and the same
	// address printed by the above call should be printed now.
	os.Args = []string{
		"siakg",
		"keyinfo",
		filepath.Join(defaultsDir, DefaultAddressName+"_Key0"+FileExtension),
	}
	main()

	// Try to create a set of keys that are invalid. An error should be printed.
	errDir := filepath.Join(testDir, "err")
	os.Args = []string{
		"siakg",
		"-f",
		errDir,
		"-t",
		"0",
	}
	main()

	// Print the key info for keys that don't exist.
	os.Args = []string{
		"siakg",
		"keyinfo",
		"notExist",
	}
	main()

	// Supply too few arguments to keyinfo.
	os.Args = []string{
		"siakg",
		"keyinfo",
	}
	main()
}
