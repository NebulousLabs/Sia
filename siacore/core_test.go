package siacore

import (
	"math/big"
	"testing"

	"github.com/NebulousLabs/Andromeda/consensus"
	"github.com/NebulousLabs/Andromeda/network"
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
	te.e0, err = CreateEnvironment(9988, 9980, true)
	if err != nil {
		te.t.Fatal(err)
	}
	te.e1, err = CreateEnvironment(9989, 9981, false)
	if err != nil {
		te.t.Fatal(err)
	}

	// Create host settings for each environment.
	defaultSettings := HostAnnouncement{
		MinFilesize:        1024,
		MaxFilesize:        1024 * 1024,
		MinDuration:        10,
		MaxDuration:        1000,
		MinChallengeWindow: 20,
		MaxChallengeWindow: 1000,
		MinTolerance:       5,
		Price:              5,
		Burn:               5,
	}

	// Create some host settings.
	te.e0.host.Settings = defaultSettings
	te.e0.host.Settings.IPAddress = network.NetAddress{"localhost", 9988}
	te.e0.host.Settings.CoinAddress = te.e0.CoinAddress()
	te.e1.host.Settings = defaultSettings
	te.e1.host.Settings.IPAddress = network.NetAddress{"localhost", 9989}
	te.e1.host.Settings.CoinAddress = te.e1.CoinAddress()

	// Give some funds to e0 and e1.
	te.e0.mineSingleBlock()
	te.e1.mineSingleBlock()

	return
}

/*
// TestSiad uses a testing environment and runs a series of tests designed to
// probe all of the siad functions and stress test siad.
func TestSiad(t *testing.T) {
	// Alter the constants to create a system more friendly to testing.
	IterationsPerAttempt = 500 * 1000
	consensus.BlockFrequency = consensus.Timestamp(1)
	consensus.TargetWindow = consensus.BlockHeight(2000)
	network.BootstrapPeers = []network.NetAddress{{"localhost", 9988}, {"localhost", 9989}}
	consensus.RootTarget[1] = 8
	consensus.MaxAdjustmentUp = big.NewRat(1001, 1000)
	consensus.MaxAdjustmentDown = big.NewRat(999, 1000)
	consensus.DEBUG = true

	if !testing.Short() {
		// Create the testing environment.
		te := establishTestingEnvironment(t)

		// Perform a series of tests using the environment.
		testToggleMining(te)
		testDualMining(te)
		testTransactionSending(te)
		testLargeTransactions(te)
		testHostDatabase(te)
	}
}
*/
