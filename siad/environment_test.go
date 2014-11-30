package siad

import (
	"testing"

	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

// A state that can be passed between functions to test the various parts of
// Sia.
type testEnv struct {
	t *testing.T

	e0 *Environment
	e1 *Environment
}

func establishTestingEnvironment(t *testing.T) (te *testEnv) {
	te = new(testEnv)
	te.t = t

	// Create two environments and mine a handful of blocks in each, verifying
	// that each got all the same blocks as the other.
	var err error
	te.e0, err = CreateEnvironment()
	if err != nil {
		te.t.Fatal(err)
	}
	te.e1, err = CreateEnvironment()
	if err != nil {
		te.t.Fatal(err)
	}

	return
}

func TestSia(t *testing.T) {
	// CreateEnvironment takes 3s for some reason.
	if testing.Short() {
		t.Skip()
	}

	// Alter the constants to create a system more friendly to testing.
	siacore.BlockFrequency = siacore.Timestamp(1)
	siacore.TargetWindow = siacore.BlockHeight(2000)
	network.BootstrapPeers = []network.NetAddress{{"localhost", 9988}, {"localhost", 9989}}
	siacore.RootTarget[0] = 4
	siacore.DEBUG = true

	// Create the testing environment.
	te := establishTestingEnvironment(t)

	// Perform a series of tests using the environment.
	testToggleMining(te)
	testDualMining(te)
}
