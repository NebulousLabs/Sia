package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
)

// The tests in this file need to be checked manually, even if errors are
// returned it will not be apparent to the test suite.

// TestMain tries running the main executable using a few different commands.
func TestMain(t *testing.T) {
	// Get a test folder.
	testDir := build.TempDir("siag", "TestMain")

	// Create a default set of keys. The result should be 'DefaultTotalKeys'
	// files being created.
	defaultsDir := filepath.Join(testDir, "defaults")
	os.Args = []string{
		"siag",
		"-f",
		defaultsDir,
	}
	main()

	// Print out the value for one of those keys, the result should be a key
	// that is 'DefaultRequiredKeys' of 'DefaultTotalKeys', and the same
	// address printed by the above call should be printed now.
	os.Args = []string{
		"siag",
		"keyinfo",
		filepath.Join(defaultsDir, DefaultAddressName+"_Key0"+FileExtension),
	}
	main()

	// Try to create a set of keys that are invalid. An error should be printed.
	errDir := filepath.Join(testDir, "err")
	os.Args = []string{
		"siag",
		"-f",
		errDir,
		"-t",
		"0",
	}
	main()

	// Print the key info for keys that don't exist.
	os.Args = []string{
		"siag",
		"keyinfo",
		"notExist",
	}
	main()

	// Supply too few arguments to keyinfo.
	os.Args = []string{
		"siag",
		"keyinfo",
	}
	main()
}
