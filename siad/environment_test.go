package siad

import (
	"math/big"
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

// establishTestingEnvrionment sets all of the testEnv variables.
func establishTestingEnvironment(t *testing.T) (te *testEnv) {
	te = new(testEnv)
	te.t = t

	// Create two environments and mine a handful of blocks in each, verifying
	// that each got all the same blocks as the other.
	var err error
	te.e0, err = CreateEnvironment(9988)
	if err != nil {
		te.t.Fatal(err)
	}
	te.e1, err = CreateEnvironment(9989)
	if err != nil {
		te.t.Fatal(err)
	}

	// Give some funds to e0 and e1.
	te.e0.mineSingleBlock()
	te.e1.mineSingleBlock()

	return
}

// TestSiad uses a testing environment and runs a series of tests designed to
// probe all of the siad functions and stress test siad.
func TestSiad(t *testing.T) {
	// Alter the constants to create a system more friendly to testing.
	siacore.BlockFrequency = siacore.Timestamp(1)
	siacore.TargetWindow = siacore.BlockHeight(2000)
	network.BootstrapPeers = []network.NetAddress{{"localhost", 9988}, {"localhost", 9989}}
	siacore.RootTarget[0] = 255
	siacore.MaxAdjustmentUp = big.NewRat(1001, 1000)
	siacore.MaxAdjustmentDown = big.NewRat(999, 1000)
	siacore.DEBUG = true

	// Create the testing environment.
	te := establishTestingEnvironment(t)

	// Perform a series of tests using the environment.
	if !testing.Short() {
		testToggleMining(te)
		testDualMining(te)
		testTransactionSending(te)
		testLargeTransactions(te)
	}
}
