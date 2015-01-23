package main

/*
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
	network.BootstrapPeers = []network.Address{"localhost:9988"}
	consensus.RootTarget[0] = 255
	consensus.MaxAdjustmentUp = big.NewRat(1005, 1000)
	consensus.MaxAdjustmentDown = big.NewRat(995, 1000)

	// Pull together the configuration for the Core.
	state := consensus.CreateGenesisState() // The missing piece is not of type error. TODO: That missing piece is deprecated.
	walletFilename := "test.wallet"
	Wallet, err := wallet.New(state, walletFilename)
	if err != nil {
		t.Fatal(err)
	}
	Miner, err := miner.New(state, Wallet)
	if err != nil {
		t.Fatal(err)
	}
	hdb, err := hostdb.New()
	if err != nil {
		t.Fatal(err)
	}
	Host, err := host.New(state, Wallet)
	if err != nil {
		return
	}
	Renter, err := renter.New(state, hdb, Wallet)
	if err != nil {
		return
	}
	coreConfig := Config{
		HostDir:     "hostdir",
		WalletFile:  walletFilename,
		ServerAddr:  ":9988",
		Nobootstrap: true,

		State: state,

		Host:   Host,
		HostDB: hdb,
		Miner:  Miner,
		Renter: Renter,
		Wallet: Wallet,
	}

	// Create the core.
	c, err = CreateCore(coreConfig)
	if err != nil {
		t.Fatal(err)
	}

	return
}

func TestEverything(t *testing.T) {
	establishTestingEnvironment(t)
	// testEmptyBlock(t, c)
	// testTransactionBlock(t, c)
	// testSendToSelf(t, c)
	// testWalletInfo(t, c)
	// testHostAnnouncement(t, c)
	// testUploadFile(t, c)
	// sendManyTransactions(t, c)
	// testMinerDeadlocking(t, c)
}

func (c *Core) ScanMutexes() {
	var state, host, hostdb, miner, renter, wallet int
	go func() {
		c.state.Height()
		state++
	}()
	go func() {
		c.host.HostInfo()
		host++
	}()
	go func() {
		c.hostDB.Size()
		hostdb++
	}()
	go func() {
		c.miner.Threads()
		miner++
	}()
	go func() {
		c.renter.RentInfo()
		renter++
	}()
	go func() {
		c.wallet.Balance(false)
		wallet++
	}()

	go func() {
		fmt.Println("mutex testing has started.")
		time.Sleep(time.Second * 10)
		fmt.Println("mutext testing results (0 means deadlock, 1 means success):")
		fmt.Println("State: ", state)
		fmt.Println("Host: ", host)
		fmt.Println("HostDB: ", hostdb)
		fmt.Println("Miner: ", miner)
		fmt.Println("Renter: ", renter)
		fmt.Println("Wallet: ", wallet)
	}()
}
*/
