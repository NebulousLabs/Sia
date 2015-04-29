package main

import (
	"fmt"
	"path/filepath"
	"runtime"

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

	"github.com/spf13/cobra"
)

// startDaemonCmd uses the config parameters to start siad.
func startDaemonCmd(*cobra.Command, []string) {
	// Establish multithreading.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Create all of the modules.
	gateway, err := gateway.New(config.Siad.RPCaddr, filepath.Join(config.Siad.SiaDir, "gateway"))
	if err != nil {
		fmt.Println("Could not start daemon:", err)
		return
	}
	state, err := consensus.New(gateway, filepath.Join(config.Siad.SiaDir, "consensus"))
	if err != nil {
		fmt.Println("Could not start daemon:", err)
		return
	}
	tpool, err := transactionpool.New(state, gateway)
	if err != nil {
		fmt.Println("Could not start daemon:", err)
		return
	}
	wallet, err := wallet.New(state, tpool, filepath.Join(config.Siad.SiaDir, "wallet"))
	if err != nil {
		fmt.Println("Could not start daemon:", err)
		return
	}
	miner, err := miner.New(state, tpool, wallet)
	if err != nil {
		fmt.Println("Could not start daemon:", err)
		return
	}
	host, err := host.New(state, tpool, wallet, config.Siad.HostAddr, filepath.Join(config.Siad.SiaDir, "host"))
	if err != nil {
		fmt.Println("Could not start daemon:", err)
		return
	}
	hostdb, err := hostdb.New(state, gateway)
	if err != nil {
		fmt.Println("Could not start daemon:", err)
		return
	}
	renter, err := renter.New(state, hostdb, wallet, filepath.Join(config.Siad.SiaDir, "renter"))
	if err != nil {
		fmt.Println("Could not start daemon:", err)
		return
	}
	srv, err := api.NewServer(config.Siad.APIaddr, state, gateway, host, hostdb, miner, renter, tpool, wallet)
	if err != nil {
		fmt.Println("Could not start daemon:", err)
		return
	}

	// Bootstrap to the network.
	if !config.Siad.NoBootstrap {
		go gateway.Bootstrap(modules.BootstrapPeers[0])
	}

	// Send a struct down the started channel, so the testing package knows
	// that daemon startup has completed.
	started <- struct{}{}

	// Start serving api requests.
	err = srv.Serve()
	if err != nil {
		fmt.Println("Could not start daemon:", err)
		return
	}
	return
}
