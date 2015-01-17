package sia

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	"github.com/NebulousLabs/Sia/sia/components"
)

// The config struct is used when calling CreateCore(), and prevents the input
// line from being excessively long.
type Config struct {
	// The State, which is separate from a componenent as it is not an
	// interface. There is a single implementation which is considered
	// acceptible.
	State *consensus.State

	// Interface implementations.
	Host   components.Host
	HostDB components.HostDB
	Miner  components.Miner
	Renter components.Renter
	Wallet components.Wallet

	// Settings available through flags.
	//
	// TODO: Most of these should be deprecated as inputs to the core - each
	// component should manage its own settings.
	HostDir     string
	WalletFile  string
	ServerAddr  string
	Nobootstrap bool
}

// Core is the struct that serves as the state for siad. It contains a
// pointer to the state, as things like a wallet, a friend list, etc. Each
// environment should have its own state.
type Core struct {
	state *consensus.State

	server *network.TCPServer // one of these things is not like the others :)
	host   components.Host
	hostDB components.HostDB
	miner  components.Miner
	renter components.Renter
	wallet components.Wallet

	// friends map[string]consensus.CoinAddress

	// Channels for incoming blocks and transactions to be processed
	blockChan       chan consensus.Block
	transactionChan chan consensus.Transaction

	// Envrionment directories.
	hostDir    string
	styleDir   string
	walletFile string
}

// createCore creates a server, host, miner, renter and wallet and
// puts it all in a single environment struct that's used as the state for the
// main package.
func CreateCore(config Config) (c *Core, err error) {
	if config.State == nil {
		err = errors.New("cannot have nil state")
		return
	}
	if config.Host == nil {
		err = errors.New("cannot have nil host")
		return
	}
	if config.HostDB == nil {
		err = errors.New("cannot have nil hostdb")
		return
	}
	if config.Miner == nil {
		err = errors.New("cannot have nil miner")
		return
	}
	if config.Renter == nil {
		err = errors.New("cannot have nil renter")
		return
	}
	if config.Wallet == nil {
		err = errors.New("cannot have nil wallet")
		return
	}

	// Set the number of procs equal to the number of cpus.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Fill out the basic information.
	c = &Core{
		state: config.State,

		host:   config.Host,
		hostDB: config.HostDB,
		miner:  config.Miner,
		renter: config.Renter,
		wallet: config.Wallet,

		// friends:         make(map[string]consensus.CoinAddress),

		blockChan:       make(chan consensus.Block, 100),
		transactionChan: make(chan consensus.Transaction, 100),

		hostDir:    config.HostDir,
		walletFile: config.WalletFile,
	}
	// TODO: The host object should be managing this on its own.
	err = os.MkdirAll(c.hostDir, os.ModeDir|os.ModePerm)
	if err != nil {
		return
	}

	// TODO: Figure out if there's any way that we need to sync to the state.
	// Create a state.
	var genesisOutputDiffs []consensus.OutputDiff
	_, genesisOutputDiffs = consensus.CreateGenesisState()
	genesisBlock, err := c.state.BlockAtHeight(0)
	if err != nil {
		return
	}

	// Update componenets to see genesis block.
	hostInfo, err := c.host.HostInfo()
	if err != nil {
		return
	}
	err = c.hostDB.Update(0, nil, []consensus.Block{genesisBlock})
	if err != nil {
		return
	}
	err = c.UpdateMiner(c.miner.Threads())
	if err != nil {
		return
	}
	err = c.wallet.Update(genesisOutputDiffs)
	if err != nil {
		return
	}

	// Bootstrap to the network (may take a few seconds).
	err = c.initializeNetwork(config.ServerAddr, config.Nobootstrap)
	if err == network.ErrNoPeers {
		fmt.Println("Warning: no peers responded to bootstrap request. Add peers manually to enable bootstrapping.")
	} else if err != nil {
		return
	}

	// TODO: Move this back up or something. The defaults are all being set in
	// weird hacky places.
	err = c.UpdateHost(hostInfo.Announcement)
	if err != nil {
		return
	}

	return
}

func (c *Core) ScanMutexes() {
	var state, host, hostdb, miner, renter, wallet int
	go func() {
		c.state.Lock()
		c.state.Unlock()
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
		c.wallet.WalletInfo()
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

// Close does any finishing maintenence before the environment can be garbage
// collected. Right now that just means closing the server.
func (c *Core) Close() {
	c.server.Close()
}
