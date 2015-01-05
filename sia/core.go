package sia

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	"github.com/NebulousLabs/Sia/sia/host"
	"github.com/NebulousLabs/Sia/sia/hostdb"
	"github.com/NebulousLabs/Sia/sia/miner"
	"github.com/NebulousLabs/Sia/sia/wallet"
)

// CoreInput is just to prevent the inputs to CreateCore() from being
// excessively long. Potentially it could be used as a struct within Core
// instead of manually copying over all of the inputs.
type Config struct {
	// Settings available through flags.
	HostDir     string
	WalletFile  string
	ServerAddr  string
	Nobootstrap bool

	// Interface implementations.
	Host   host.Host
	HostDB hostdb.HostDB
	Miner  miner.Miner
	Wallet wallet.Wallet
}

// Core is the struct that serves as the state for siad. It contains a
// pointer to the state, as things like a wallet, a friend list, etc. Each
// environment should have its own state.
type Core struct {
	state *consensus.State

	server *network.TCPServer
	host   host.Host
	hostDB hostdb.HostDB
	miner  miner.Miner
	wallet wallet.Wallet

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
	if config.Wallet == nil {
		err = errors.New("cannot have nil wallet")
		return
	}

	// Fill out the basic information.
	c = &Core{
		host:   config.Host,
		hostDB: config.HostDB,
		miner:  config.Miner,
		wallet: config.Wallet,

		// friends:         make(map[string]consensus.CoinAddress),

		blockChan:       make(chan consensus.Block, 100),
		transactionChan: make(chan consensus.Transaction, 100),

		hostDir:    config.HostDir,
		walletFile: config.WalletFile,
	}

	// Create a state.
	var genesisOutputDiffs []consensus.OutputDiff
	c.state, genesisOutputDiffs = consensus.CreateGenesisState()

	// Update componenets to see genesis block.
	err = c.UpdateMiner(0)
	if err != nil {
		return
	}
	err = c.wallet.Update(genesisOutputDiffs)
	if err != nil {
		return
	}

	/*
		// c.host = CreateHost()
		c.hostDB = hostdb.New()
		c.miner = miner.New(c.blockChan, 1)
		// c.renter = CreateRenter()
		c.wallet, err = wallet.New(c.walletFile)
		if err != nil {
			return
		}

	*/

	// Bootstrap to the network.
	err = c.initializeNetwork(config.ServerAddr, config.Nobootstrap)
	if err == network.ErrNoPeers {
		// log.Println("Warning: no peers responded to bootstrap request. Add peers manually to enable bootstrapping.")
	} else if err != nil {
		return
	}
	// c.host.Settings.IPAddress = c.server.Address()

	return
}

// Close does any finishing maintenence before the environment can be garbage
// collected. Right now that just means closing the server.
func (c *Core) Close() {
	c.server.Close()
}
