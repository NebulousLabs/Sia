package main

import (
	"path/filepath"

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
)

// DaemonConfig is a struct containing the daemon configuration variables. It
// is only used when calling 'newDaemon', but is it's own struct because there
// are many values.
type DaemonConfig struct {
	APIAddr string
	RPCAddr string

	SiaDir string
}

// The daemon is essentially a collection of modules and an API server to talk
// to them all.
type daemon struct {
	state   *consensus.State
	gateway modules.Gateway
	host    modules.Host
	hostdb  modules.HostDB
	miner   *miner.Miner
	renter  modules.Renter
	tpool   modules.TransactionPool
	wallet  modules.Wallet

	apiServer *graceful.Server
}

// newDaemon will take the config struct and create a new daemon based on the
// parameters.
func newDaemon(config DaemonConfig) (d *daemon, err error) {
	d = new(daemon)
	d.state = consensus.CreateGenesisState()
	d.gateway, err = gateway.New(config.RPCAddr, d.state, filepath.Join(config.SiaDir, "gateway"))
	if err != nil {
		return
	}
	d.tpool, err = transactionpool.New(d.state, d.gateway)
	if err != nil {
		return
	}
	d.wallet, err = wallet.New(d.state, d.tpool, filepath.Join(config.SiaDir, "wallet"))
	if err != nil {
		return
	}
	d.miner, err = miner.New(d.state, d.gateway, d.tpool, d.wallet)
	if err != nil {
		return
	}
	d.host, err = host.New(d.state, d.tpool, d.wallet, filepath.Join(config.SiaDir, "host"))
	if err != nil {
		return
	}
	d.hostdb, err = hostdb.New(d.state, d.gateway)
	if err != nil {
		return
	}
	d.renter, err = renter.New(d.state, d.gateway, d.hostdb, d.wallet, filepath.Join(config.SiaDir, "renter"))
	if err != nil {
		return
	}

	// Register RPCs for each module
	d.gateway.RegisterRPC("RelayBlock", d.relayBlock)
	d.gateway.RegisterRPC("AcceptTransaction", d.acceptTransaction)
	d.gateway.RegisterRPC("HostSettings", d.host.Settings)
	d.gateway.RegisterRPC("NegotiateContract", d.host.NegotiateContract)
	d.gateway.RegisterRPC("RetrieveFile", d.host.RetrieveFile)

	d.initAPI(config.APIAddr)

	return
}

// TODO: move this to the state module?
func (d *daemon) relayBlock(conn modules.NetConn) error {
	var b consensus.Block
	err := conn.ReadObject(&b, consensus.BlockSizeLimit)
	if err != nil {
		return err
	}
	return d.gateway.RelayBlock(b)
}

// TODO: move this to the tpool module?
func (d *daemon) acceptTransaction(conn modules.NetConn) error {
	var t consensus.Transaction
	err := conn.ReadObject(&t, consensus.BlockSizeLimit)
	if err != nil {
		return err
	}
	return d.tpool.AcceptTransaction(t)
}
