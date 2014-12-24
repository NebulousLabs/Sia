package siacore

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	"github.com/NebulousLabs/Sia/siacore/miner"
	"github.com/NebulousLabs/Sia/siacore/wallet"
)

// Environment is the struct that serves as the state for siad. It contains a
// pointer to the state, as things like a wallet, a friend list, etc. Each
// environment should have its own state.
type Environment struct {
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

// createEnvironment creates a server, host, miner, renter and wallet and
// puts it all in a single environment struct that's used as the state for the
// main package.
//
// TODO: swap out the way that CreateEnvironment is called so that the wallet,
// host, etc. can all be used as input - or not supplied at all.
func CreateEnvironment(hostDir string, walletFile string, serverAddr string, nobootstrap bool) (e *Environment, err error) {
	e = &Environment{
		friends:         make(map[string]consensus.CoinAddress),
		blockChan:       make(chan consensus.Block, 100),
		transactionChan: make(chan consensus.Transaction, 100),
		hostDir:         hostDir,
		walletFile:      walletFile,
	}
	var genesisOutputDiffs []consensus.OutputDiff
	e.state, genesisOutputDiffs = consensus.CreateGenesisState()
	e.hostDatabase = CreateHostDatabase()
	e.host = CreateHost()
	e.miner = miner.New(e.blockChan, 1)
	e.renter = CreateRenter()
	e.wallet, err = wallet.New(e.walletFile)
	if err != nil {
		return
	}

	// Update componenets to see genesis block.
	err = e.updateMiner(e.miner)
	if err != nil {
		return
	}
	err = e.wallet.Update(genesisOutputDiffs)
	if err != nil {
		return
	}

	// Bootstrap to the network.
	err = e.initializeNetwork(serverAddr, nobootstrap)
	if err == network.ErrNoPeers {
		// log.Println("Warning: no peers responded to bootstrap request. Add peers manually to enable bootstrapping.")
	} else if err != nil {
		return
	}
	e.host.Settings.IPAddress = e.server.Address()

	return
}

// Close does any finishing maintenence before the environment can be garbage
// collected. Right now that just means closing the server.
func (e *Environment) Close() {
	e.server.Close()
}
