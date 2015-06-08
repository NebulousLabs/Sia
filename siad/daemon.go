package main

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/blockexplorer"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/hostdb"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"

	"github.com/spf13/cobra"
)

// startDaemonCmd uses the config parameters to start siad.
func startDaemon() error {
	// Establish multithreading.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Create all of the modules.
	gateway, err := gateway.New(config.Siad.RPCaddr, filepath.Join(config.Siad.SiaDir, "gateway"))
	if err != nil {
		return err
	}
	state, err := consensus.New(gateway, filepath.Join(config.Siad.SiaDir, "consensus"))
	if err != nil {
		return err
	}
	tpool, err := transactionpool.New(state, gateway)
	if err != nil {
		return err
	}
	wallet, err := wallet.New(state, tpool, filepath.Join(config.Siad.SiaDir, "wallet"))
	if err != nil {
		return err
	}
	miner, err := miner.New(state, tpool, wallet)
	if err != nil {
		return err
	}
	hostdb, err := hostdb.New(state, gateway)
	if err != nil {
		return err
	}
	host, err := host.New(state, hostdb, tpool, wallet, config.Siad.HostAddr, filepath.Join(config.Siad.SiaDir, "host"))
	if err != nil {
		return err
	}
	renter, err := renter.New(state, hostdb, wallet, filepath.Join(config.Siad.SiaDir, "renter"))
	if err != nil {
		return err
	}
	explorer, err := blockexplorer.New(state)
	if err != nil {
		return err
	}
	srv, err := api.NewServer(config.Siad.APIaddr, state, gateway, host, hostdb, miner, renter, tpool, wallet, explorer)
	if err != nil {
		return err
	}

	// Bootstrap to the network.
	if !config.Siad.NoBootstrap {
		for i := range modules.BootstrapPeers {
			go gateway.Connect(modules.BootstrapPeers[i])
		}
	}

	// Send a struct down the started channel, so the testing package knows
	// that daemon startup has completed. A gofunc is used with the hope that
	// srv.Serve() will start running before the value is sent down the
	// channel.
	go func() {
		started <- struct{}{}
	}()

	// Start serving api requests.
	err = srv.Serve()
	if err != nil {
		return err
	}
	return nil
}

// startDaemonCmd is a passthrough function for startDaemon.
func startDaemonCmd(*cobra.Command, []string) {
	err := startDaemon()
	if err != nil {
		fmt.Println(err)
	}
}
