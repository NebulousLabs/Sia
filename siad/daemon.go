package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/explorer"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/host"
	"github.com/NebulousLabs/Sia/modules/miner"
	"github.com/NebulousLabs/Sia/modules/renter"
	"github.com/NebulousLabs/Sia/modules/transactionpool"
	"github.com/NebulousLabs/Sia/modules/wallet"
	"github.com/NebulousLabs/Sia/profile"

	"github.com/spf13/cobra"
)

// processNetAddr adds a ':' to a bare integer, so that it is a proper port
// number.
func processNetAddr(addr string) string {
	_, err := strconv.Atoi(addr)
	if err == nil {
		return ":" + addr
	}
	return addr
}

// processConfig checks the configuration values and performs cleanup on
// incorrect-but-allowed values.
func processConfig(config Config) Config {
	config.Siad.APIaddr = processNetAddr(config.Siad.APIaddr)
	config.Siad.RPCaddr = processNetAddr(config.Siad.RPCaddr)
	config.Siad.HostAddr = processNetAddr(config.Siad.HostAddr)
	return config
}

// startDaemonCmd uses the config parameters to start siad.
func startDaemon(config Config) error {
	// Print a startup message.
	fmt.Println("Loading...")
	loadStart := time.Now()

	// Create all of the modules.
	gateway, err := gateway.New(config.Siad.RPCaddr, filepath.Join(config.Siad.SiaDir, modules.GatewayDir))
	if err != nil {
		return err
	}
	cs, err := consensus.New(gateway, filepath.Join(config.Siad.SiaDir, modules.ConsensusDir))
	if err != nil {
		return err
	}
	var e *explorer.Explorer
	if config.Siad.Explorer {
		e, err = explorer.New(cs, filepath.Join(config.Siad.SiaDir, modules.ExplorerDir))
		if err != nil {
			return err
		}
	}
	tpool, err := transactionpool.New(cs, gateway)
	if err != nil {
		return err
	}
	wallet, err := wallet.New(cs, tpool, filepath.Join(config.Siad.SiaDir, modules.WalletDir))
	if err != nil {
		return err
	}
	miner, err := miner.New(cs, tpool, wallet, filepath.Join(config.Siad.SiaDir, modules.MinerDir))
	if err != nil {
		return err
	}
	host, err := host.New(cs, tpool, wallet, config.Siad.HostAddr, filepath.Join(config.Siad.SiaDir, modules.HostDir))
	if err != nil {
		return err
	}
	renter, err := renter.New(cs, wallet, tpool, filepath.Join(config.Siad.SiaDir, modules.RenterDir))
	if err != nil {
		return err
	}
	srv, err := api.NewServer(
		config.Siad.APIaddr,
		config.Siad.RequiredUserAgent,
		cs,
		e,
		gateway,
		host,
		miner,
		renter,
		tpool,
		wallet,
	)
	if err != nil {
		return err
	}

	// Bootstrap to the network.
	if !config.Siad.NoBootstrap {
		// connect to 3 random bootstrap nodes
		perm := crypto.Perm(len(modules.BootstrapPeers))
		for _, i := range perm[:3] {
			go gateway.Connect(modules.BootstrapPeers[i])
		}
	}

	// Print a 'startup complete' message.
	startupTime := time.Since(loadStart)
	fmt.Println("Finished loading in", startupTime.Seconds(), "seconds")

	// Start serving api requests.
	err = srv.Serve()
	if err != nil {
		return err
	}
	return nil
}

// startDaemonCmd is a passthrough function for startDaemon.
func startDaemonCmd(*cobra.Command, []string) {
	// Create the profiling directory if profiling is enabled.
	if globalConfig.Siad.Profile {
		go profile.StartContinuousProfile(globalConfig.Siad.ProfileDir)
	}

	// Process the config variables after they are parsed by cobra.
	config := processConfig(globalConfig)

	// Start siad. startDaemon will only return when it is shutting down.
	err := startDaemon(config)
	if err != nil {
		fmt.Println(err)
	}
}
