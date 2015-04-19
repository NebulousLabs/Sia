package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/modules/tester"
)

// The tests in this file need to be checked manually, even if errors are
// returned it will not be apparent to the test suite.
//
// CONTRIBUTE: Make these tests automatic.

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
		"-f",
		filepath.Join(defaultsDir, DefaultKeyname+"_Key0"+FileExtension),
	}
	main()

	// Attempt to create an address with 0 required signatures. zeroRequiredDir
	// should not be created because of an error.
	zeroRequiredDir := filepath.Join(testDir, "zeroRequired")
	os.Args = []string{
		"siakg",
		"-f",
		zeroRequiredDir,
		"-r",
		"0",
	}
	main()

	// Attempt to create an unspendable address. unspendableDir should not be
	// created because of an error.
	unspendableDir := filepath.Join(testDir, "zeroRequired")
	os.Args = []string{
		"siakg",
		"-f",
		unspendableDir,
		"-t",
		"1",
	}
	main()

	// Attempt read a nonexistant file. The output should be a nonexistant file
	// error.
	os.Args = []string{
		"siakg",
		"keyinfo",
		"-f",
		"idontexist",
	}
	main()

	// Attempt to read a corrupted file. The output should be something
	// indicating corruption.
	os.Args = []string{
		"siakg",
		"keyinfo",
		"-f",
		"main.go",
	}
	main()
}
