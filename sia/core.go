package sia

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	"github.com/NebulousLabs/Sia/sia/miner"
	"github.com/NebulousLabs/Sia/sia/wallet"
)

// Core is the struct that serves as the state for siad. It contains a
// pointer to the state, as things like a wallet, a friend list, etc. Each
// environment should have its own state.
type Core struct {
	state *consensus.State

	server       *network.TCPServer
	host         *Host
	hostDatabase *HostDatabase
	miner        Miner
	renter       *Renter
	wallet       Wallet

	friends map[string]consensus.CoinAddress

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
//
// TODO: swap out the way that CreateCore is called so that the wallet,
// host, etc. can all be used as input - or not supplied at all.
func CreateCore(hostDir string, walletFile string, serverAddr string, nobootstrap bool) (c *Core, err error) {
	c = &Core{
		friends:         make(map[string]consensus.CoinAddress),
		blockChan:       make(chan consensus.Block, 100),
		transactionChan: make(chan consensus.Transaction, 100),
		hostDir:         hostDir,
		walletFile:      walletFile,
	}
	var genesisOutputDiffs []consensus.OutputDiff
	c.state, genesisOutputDiffs = consensus.CreateGenesisState()
	c.hostDatabase = CreateHostDatabase()
	c.host = CreateHost()
	c.miner = miner.New(c.blockChan, 1)
	c.renter = CreateRenter()
	c.wallet, err = wallet.New(c.walletFile)
	if err != nil {
		return
	}

	// Update componenets to see genesis block.
	err = c.updateMiner(c.miner)
	if err != nil {
		return
	}
	err = c.wallet.Update(genesisOutputDiffs)
	if err != nil {
		return
	}

	// Bootstrap to the network.
	err = c.initializeNetwork(serverAddr, nobootstrap)
	if err == network.ErrNoPeers {
		// log.Println("Warning: no peers responded to bootstrap request. Add peers manually to enable bootstrapping.")
	} else if err != nil {
		return
	}
	c.host.Settings.IPAddress = c.server.Address()

	return
}

// Close does any finishing maintenence before the environment can be garbage
// collected. Right now that just means closing the server.
func (c *Core) Close() {
	c.server.Close()
}
