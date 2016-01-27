package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
func processConfig(config Config) (Config, error) {
	config.Siad.APIaddr = processNetAddr(config.Siad.APIaddr)
	config.Siad.RPCaddr = processNetAddr(config.Siad.RPCaddr)
	config.Siad.HostAddr = processNetAddr(config.Siad.HostAddr)
	config.Siad.Modules = strings.ToLower(config.Siad.Modules)
	validModules := "cghmrtwe"
	invalidModules := config.Siad.Modules
	for _, m := range validModules {
		invalidModules = strings.Replace(invalidModules, string(m), "", 1)
	}
	if len(invalidModules) > 0 {
		return Config{}, errors.New("Unable to parse --modules flag, unrecognized modules: " + invalidModules)
	}
	return config, nil
}

// startDaemonCmd uses the config parameters to start siad.
func startDaemon(config Config) (err error) {
	// Print a startup message.
	fmt.Println("Loading...")
	loadStart := time.Now()

	// Create all of the modules.
	var g modules.Gateway
	if strings.Contains(config.Siad.Modules, "g") {
		fmt.Println("Loading gateway...")
		g, err = gateway.New(config.Siad.RPCaddr, filepath.Join(config.Siad.SiaDir, modules.GatewayDir))
		if err != nil {
			return err
		}
	}
	var cs modules.ConsensusSet
	if strings.Contains(config.Siad.Modules, "c") {
		fmt.Println("Loading consensus...")
		cs, err = consensus.New(g, filepath.Join(config.Siad.SiaDir, modules.ConsensusDir))
		if err != nil {
			return err
		}
	}
	var e modules.Explorer
	if strings.Contains(config.Siad.Modules, "e") {
		fmt.Println("Loading explorer...")
		e, err = explorer.New(cs, filepath.Join(config.Siad.SiaDir, modules.ExplorerDir))
		if err != nil {
			return err
		}
	}
	var tpool modules.TransactionPool
	if strings.Contains(config.Siad.Modules, "t") {
		fmt.Println("Loading transaction pool...")
		tpool, err = transactionpool.New(cs, g)
		if err != nil {
			return err
		}
	}
	var w modules.Wallet
	if strings.Contains(config.Siad.Modules, "w") {
		fmt.Println("Loading wallet...")
		w, err = wallet.New(cs, tpool, filepath.Join(config.Siad.SiaDir, modules.WalletDir))
		if err != nil {
			return err
		}
	}
	var m modules.Miner
	if strings.Contains(config.Siad.Modules, "m") {
		fmt.Println("Loading miner...")
		m, err = miner.New(cs, tpool, w, filepath.Join(config.Siad.SiaDir, modules.MinerDir))
		if err != nil {
			return err
		}
	}
	var h modules.Host
	if strings.Contains(config.Siad.Modules, "h") {
		fmt.Println("Loading host...")
		h, err = host.New(cs, tpool, w, config.Siad.HostAddr, filepath.Join(config.Siad.SiaDir, modules.HostDir))
		if err != nil {
			return err
		}
	}
	var r modules.Renter
	if strings.Contains(config.Siad.Modules, "r") {
		fmt.Println("Loading renter...")
		r, err = renter.New(cs, w, tpool, filepath.Join(config.Siad.SiaDir, modules.RenterDir))
		if err != nil {
			return err
		}
	}
	srv, err := api.NewServer(
		config.Siad.APIaddr,
		config.Siad.RequiredUserAgent,
		cs,
		e,
		g,
		h,
		m,
		r,
		tpool,
		w,
	)
	if err != nil {
		return err
	}

	// Bootstrap to the network.
	if !config.Siad.NoBootstrap && g != nil {
		// connect to 3 random bootstrap nodes
		perm, err := crypto.Perm(len(modules.BootstrapPeers))
		if err != nil {
			return err
		}
		for _, i := range perm[:3] {
			go g.Connect(modules.BootstrapPeers[i])
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
func startDaemonCmd(cmd *cobra.Command, _ []string) {
	// Create the profiling directory if profiling is enabled.
	if globalConfig.Siad.Profile {
		go profile.StartContinuousProfile(globalConfig.Siad.ProfileDir)
	}

	// Process the config variables after they are parsed by cobra.
	config, err := processConfig(globalConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		cmd.Usage()
		os.Exit(exitCodeUsage)
	}

	// Start siad. startDaemon will only return when it is shutting down.
	err = startDaemon(config)
	if err != nil {
		die(err)
	}
}
