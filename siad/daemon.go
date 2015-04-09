package main

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
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

type daemon struct {
	srv *api.Server
}

// newDaemon initializes modules using the config parameters and uses them to
// create an api.Server.
func newDaemon(cfg DaemonConfig) (d *daemon, err error) {
	state, err := consensus.New(filepath.Join(cfg.SiaDir, "consensus"))
	if err != nil {
		return
	}
	gateway, err := gateway.New(cfg.RPCAddr, state, filepath.Join(cfg.SiaDir, "gateway"))
	if err != nil {
		return
	}
	tpool, err := transactionpool.New(state, gateway)
	if err != nil {
		return
	}
	wallet, err := wallet.New(state, tpool, filepath.Join(cfg.SiaDir, "wallet"))
	if err != nil {
		return
	}
	miner, err := miner.New(state, gateway, tpool, wallet)
	if err != nil {
		return
	}
	host, err := host.New(state, tpool, wallet, filepath.Join(cfg.SiaDir, "host"))
	if err != nil {
		return
	}
	hostdb, err := hostdb.New(state, gateway)
	if err != nil {
		return
	}
	renter, err := renter.New(state, gateway, hostdb, wallet, filepath.Join(cfg.SiaDir, "renter"))
	if err != nil {
		return
	}

	// bootstrap to the network
	// TODO: probably a better way of doing this.
	if !config.Siacore.NoBootstrap {
		go gateway.Bootstrap(modules.BootstrapPeers[0])
	}

	d = &daemon{api.NewServer(cfg.APIAddr, state, gateway, host, hostdb, miner, renter, tpool, wallet)}
	return
}
