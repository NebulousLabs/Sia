package sia

import (
	"math/big"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	"github.com/NebulousLabs/Sia/sia/host"
	"github.com/NebulousLabs/Sia/sia/hostdb"
	"github.com/NebulousLabs/Sia/sia/miner"
	"github.com/NebulousLabs/Sia/sia/renter"
	"github.com/NebulousLabs/Sia/sia/wallet"
)

// establishTestingEnvrionment sets all of the testEnv variables.
func establishTestingEnvironment(t *testing.T) (c *Core) {
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

	// Pull together the configuration for the Core.
	walletFilename := "test.wallet"
	wallet, err := wallet.New(walletFilename)
	if err != nil {
		return
	}
	coreConfig := Config{
		HostDir:     "hostdir",
		WalletFile:  walletFilename,
		ServerAddr:  ":9988",
		Nobootstrap: true,

		Host:   host.New(),
		HostDB: hostdb.New(),
		Miner:  miner.New(),
		Renter: renter.New(),
		Wallet: wallet,
	}

	// Create the core.
	c, err = CreateCore(coreConfig)
	if err != nil {
		t.Fatal(err)
	}

	return
}

func TestEverything(t *testing.T) {
	c := establishTestingEnvironment(t)
	testEmptyBlock(t, c)
	testTransactionBlock(t, c)
	testSendToSelf(t, c)
	testWalletInfo(t, c)
	testHostAnnouncement(t, c)

	// TODO: add some tests which probe the miner implementation more.
}
