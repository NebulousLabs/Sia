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

	"github.com/spf13/cobra"
)

// startDaemonCmd uses the config parameters to start siad.
func startDaemon() error {
	// Establish multithreading.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Create all of the modules.
	gateway, err := gateway.New(config.Siad.RPCaddr, filepath.Join(config.Siad.SiaDir, modules.GatewayDir))
	if err != nil {
		return err
	}
	state, err := consensus.New(gateway, filepath.Join(config.Siad.SiaDir, modules.ConsensusDir))
	if err != nil {
		return err
	}
	explorer, err := blockexplorer.New(state, filepath.Join(config.Siad.SiaDir, modules.ExplorerDir))
	if err != nil {
		return err
	}
	srv, err := api.NewServer(config.Siad.APIaddr, state, gateway, nil, nil, nil, nil, nil, nil, explorer)
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
