package siacore

import (
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
)

// establishTestingEnvrionment sets all of the testEnv variables.
func establishTestingEnvironment(t *testing.T) (e *Environment) {
	// Alter the constants to create a system more friendly to testing.
	//
	// TODO: Perhaps also have these constants as a build flag, then they don't
	// need to be variables.
	consensus.BlockFrequency = consensus.Timestamp(1)
	consensus.TargetWindow = consensus.BlockHeight(1000)
	network.BootstrapPeers = []network.Address{"localhost:9988", "localhost:9989"}
	consensus.RootTarget[0] = 255
	consensus.MaxAdjustmentUp = big.NewRat(1005, 1000)
	consensus.MaxAdjustmentDown = big.NewRat(995, 1000)

	e, err := CreateEnvironment("host", "test.wallet", ":9988", true)
	if err != nil {
		t.Fatal(err)
	}

	/*
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
	*/

	return
}

// I'm not sure how to test asynchronous code, so at this point I don't try, I
// only test the synchronous parts.
func TestEverything(t *testing.T) {
	e := establishTestingEnvironment(t)
	testEmptyBlock(t, e)
	testTransactionBlock(t, e)
	testSendToSelf(t, e)
	testWalletInfo(t, e)
}
