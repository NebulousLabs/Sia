package main

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/api"
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

type daemon struct {
	srv *api.Server
}

// newDaemon will take the config struct and create a new daemon based on the
// parameters.
func newDaemon(config DaemonConfig) (d *daemon, err error) {
	state = consensus.CreateGenesisState()
	gateway, err = gateway.New(config.RPCAddr, state, filepath.Join(config.SiaDir, "gateway"))
	if err != nil {
		return
	}
	tpool, err = transactionpool.New(state, gateway)
	if err != nil {
		return
	}
	wallet, err = wallet.New(state, tpool, filepath.Join(config.SiaDir, "wallet"))
	if err != nil {
		return
	}
	miner, err = miner.New(state, gateway, tpool, wallet)
	if err != nil {
		return
	}
	host, err = host.New(state, tpool, wallet, filepath.Join(config.SiaDir, "host"))
	if err != nil {
		return
	}
	hostdb, err = hostdb.New(state, gateway)
	if err != nil {
		return
	}
	renter, err = renter.New(state, gateway, hostdb, wallet, filepath.Join(config.SiaDir, "renter"))
	if err != nil {
		return
	}

	d = new(daemon)
	d.srv, err = api.NewServer(config.APIAddr, state, gateway, tpool, wallet, miner, host, hostdb, renter)
	if err != nil {
		return
	}
}
