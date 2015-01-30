package main

import (
	"html/template"

	"github.com/stretchr/graceful"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
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
	// Modules. TODO: Implement all modules.
	state   *consensus.State
	tpool   *transactionpool.TransactionPool
	network *network.TCPServer
	wallet  *wallet.Wallet
	miner   *miner.Miner
	host    *host.Host

	styleDir    string
	downloadDir string

	template *template.Template

	apiServer *graceful.Server
}

func newDaemon(config DaemonConfig) (d *daemon, err error) {
	d = new(daemon)
	d.state = consensus.CreateGenesisState()
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
	d.host, err = host.New(d.state, d.wallet)
	if err != nil {
		return
	}
	/*
		hostDB, err := hostdb.New()
		if err != nil {
			return
		}

			Renter, err := renter.New(d.state, hostDB, d.wallet)
			if err != nil {
				return
			}
	*/

	// register RPC handlers
	// TODO: register all RPCs in a separate function
	err = d.network.RegisterRPC("AcceptBlock", d.state.AcceptBlock)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("AcceptTransaction", d.tpool.AcceptTransaction)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("SendBlocks", d.SendBlocks)
	if err != nil {
		return
	}
	err = d.network.RegisterRPC("NegotiateContract", d.host.NegotiateContract)
	if err != nil {
		return
	}

	return
}
