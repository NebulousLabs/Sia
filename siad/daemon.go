package main

import (
	"html/template"

	"github.com/stretchr/graceful"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/hostdb"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/network"
)

type DaemonConfig struct {
	// Network Variables
	APIAddr string
	RPCAddr string

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
	state   *consensus.State
	network *network.TCPServer
	gateway modules.Gateway
	host    modules.Host
	hostdb  modules.HostDB
	miner   *miner.Miner
	renter  modules.Renter
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	styleDir    string
	downloadDir string

	template *template.Template

	apiServer *graceful.Server
}

func newDaemon(config DaemonConfig) (d *daemon, err error) {
	d = new(daemon)
	d.state = consensus.CreateGenesisState()
	d.network, err = network.NewTCPServer(config.RPCAddr)
	if err != nil {
		return
	}
	d.gateway, err = gateway.New(d.network, d.state)
	if err != nil {
		return
	}
	d.tpool, err = transactionpool.New(d.state, d.gateway)
	if err != nil {
		return
	}
	d.wallet, err = wallet.New(d.state, d.tpool, config.WalletDir)
	if err != nil {
		return
	}
	d.miner, err = miner.New(d.state, d.gateway, d.tpool, d.wallet)
	if err != nil {
		return
	}
	d.host, err = host.New(d.state, d.tpool, d.wallet, config.HostDir)
	if err != nil {
		return
	}
	d.hostdb, err = hostdb.New(d.state)
	if err != nil {
		return
	}
	d.renter, err = renter.New(d.state, d.hostdb, d.wallet)
	if err != nil {
		return
	}

	err = d.initRPC()
	if err != nil {
		return
	}
	d.initAPI(config.APIAddr)

	return
}

// initRPC registers all of the daemon's RPC handlers
func (d *daemon) initRPC() (err error) {
	err = d.network.RegisterRPC("RelayBlock", d.gateway.RelayBlock)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("AcceptTransaction", d.tpool.AcceptTransaction)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("AddMe", d.gateway.AddMe)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("SharePeers", d.gateway.SharePeers)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("SendBlocks", d.gateway.SendBlocks)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("HostSettings", d.host.Settings)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("NegotiateContract", d.host.NegotiateContract)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("RetrieveFile", d.host.RetrieveFile)
	if err != nil {
		return
	}

	return
}
