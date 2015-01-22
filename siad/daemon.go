package main

import (
	"html/template"

	"github.com/stretchr/graceful"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	"github.com/NebulousLabs/Sia/sia/host"
	"github.com/NebulousLabs/Sia/sia/hostdb"
	"github.com/NebulousLabs/Sia/sia/miner"
	"github.com/NebulousLabs/Sia/sia/renter"
	"github.com/NebulousLabs/Sia/sia/wallet"
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

	// Deprecated Stuff
	StyleDir string
}

type daemon struct {
	// Modules. TODO: Implement all modules.
	state   *consensus.State
	miner   *miner.Miner
	network *network.TCPServer
	wallet  *wallet.Wallet

	styleDir    string
	downloadDir string

	template *template.Template

	apiServer *graceful.Server
}

func startDaemon(config DaemonConfig) (d *daemon, err error) {
	d.state = consensus.CreateGenesisState()
	d.network, err = network.NewTCPServer(config.RPCAddr)
	if err != nil {
		return
	}
	d.wallet, err = wallet.New(d.state, config.WalletDir)
	if err != nil {
		return
	}
	d.miner, err = miner.New(d.state, d.wallet)
	if err != nil {
		return
	}
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

	// Begin listening for requests on the API.
	// handle will run until /stop is called or an interrupt is caught.
	d.handle(config.APIAddr)

	return
}
