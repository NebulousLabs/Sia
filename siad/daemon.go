package main

import (
	"fmt"
	"html/template"

	"github.com/stretchr/graceful"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	// "github.com/NebulousLabs/Sia/sia/host"
	// "github.com/NebulousLabs/Sia/sia/hostdb"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	// "github.com/NebulousLabs/Sia/sia/renter"
	"github.com/NebulousLabs/Sia/modules/wallet"
)

type DaemonConfig struct {
	// Network Variables
	APIAddr     string
	RPCAddr     string
	NoBootstrap bool

	// Host Variables
	HostDir string

	// Miner Variables
	Threads int

	// Renter Variables
	DownloadDir string

	// Wallet Variables
	WalletDir string
}

type daemon struct {
	// Modules. TODO: Implement all modules.
	state   *consensus.State
	tpool   *transactionpool.TransactionPool
	miner   *miner.Miner
	network *network.TCPServer
	wallet  *wallet.Wallet

	styleDir    string
	downloadDir string

	template *template.Template

	apiServer *graceful.Server
}

func newDaemon(config DaemonConfig) (d *daemon, err error) {
	d = new(daemon)
	d.state = consensus.CreateGenesisState(consensus.GenesisTimestamp)
	d.tpool, err = transactionpool.New(d.state)
	if err != nil {
		return
	}
	d.network, err = network.NewTCPServer(config.RPCAddr)
	if err != nil {
		return
	}
	d.wallet, err = wallet.New(d.state, d.tpool, config.WalletDir)
	if err != nil {
		return
	}
	d.miner, err = miner.New(d.state, d.tpool, d.wallet)
	if err != nil {
		return
	}
	/*
		hostDB, err := hostdb.New()
		if err != nil {
			return
		}
			Host, err := host.New(d.state, d.wallet)
			if err != nil {
				return
			}
			Renter, err := renter.New(d.state, hostDB, d.wallet)
			if err != nil {
				return
			}
	*/

	d.initializeNetwork(config.RPCAddr, config.NoBootstrap)
	if err == network.ErrNoPeers {
		fmt.Println("Warning: no peers responded to bootstrap request. Add peers manually to enable bootstrapping.")
	} else if err != nil {
		return
	}

	return
}
