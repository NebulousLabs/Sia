package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"os"
	"os/signal"

	"github.com/NebulousLabs/Sia/api"
	"github.com/NebulousLabs/Sia/build"
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

	"github.com/bgentry/speakeasy"
	"github.com/spf13/cobra"
)

// verifyAPISecurity checks that the security values are consistent with a
// sane, secure system.
func verifyAPISecurity(config Config) error {
	// Make sure that only the loopback address is allowed unless the
	// --disable-api-security flag has been used.
	if !config.Siad.AllowAPIBind {
		addr := modules.NetAddress(config.Siad.APIaddr)
		if !addr.IsLoopback() {
			if addr.Host() == "" {
				return fmt.Errorf("a blank host will listen on all interfaces, did you mean localhost:%v?\nyou must pass --disable-api-security to bind Siad to a non-localhost address", addr.Port())
			}
			return errors.New("you must pass --disable-api-security to bind Siad to a non-localhost address")
		}
		return nil
	}

	// If the --disable-api-security flag is used, enforce that
	// --authenticate-api must also be used.
	if config.Siad.AllowAPIBind && !config.Siad.AuthenticateAPI {
		return errors.New("cannot use --disable-api-security without setting an api password")
	}
	return nil
}

// processNetAddr adds a ':' to a bare integer, so that it is a proper port
// number.
func processNetAddr(addr string) string {
	_, err := strconv.Atoi(addr)
	if err == nil {
		return ":" + addr
	}
	return addr
}

// processModules makes the modules string lowercase to make checking if a
// module in the string easier, and returns an error if the string contains an
// invalid module character.
func processModules(modules string) (string, error) {
	modules = strings.ToLower(modules)
	validModules := "cghmrtwe"
	invalidModules := modules
	for _, m := range validModules {
		invalidModules = strings.Replace(invalidModules, string(m), "", 1)
	}
	if len(invalidModules) > 0 {
		return "", errors.New("Unable to parse --modules flag, unrecognized or duplicate modules: " + invalidModules)
	}
	return modules, nil
}

// processConfig checks the configuration values and performs cleanup on
// incorrect-but-allowed values.
func processConfig(config Config) (Config, error) {
	var err1 error
	config.Siad.APIaddr = processNetAddr(config.Siad.APIaddr)
	config.Siad.RPCaddr = processNetAddr(config.Siad.RPCaddr)
	config.Siad.HostAddr = processNetAddr(config.Siad.HostAddr)
	config.Siad.Modules, err1 = processModules(config.Siad.Modules)
	err2 := verifyAPISecurity(config)
	err := build.JoinErrors([]error{err1, err2}, ", and ")
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

// startDaemonCmd uses the config parameters to start siad.
func startDaemon(config Config) (err error) {
	// Prompt user for API password.
	if config.Siad.AuthenticateAPI {
		config.APIPassword, err = speakeasy.Ask("Enter API password: ")
		if err != nil {
			return err
		}
		if config.APIPassword == "" {
			return errors.New("password cannot be blank")
		}
	}

	// Process the config variables after they are parsed by cobra.
	config, err = processConfig(config)
	if err != nil {
		return err
	}

	// Print a startup message.
	fmt.Println("Loading...")
	loadStart := time.Now()

	// Create the server and start serving daemon routes immediately.
	i := 0
	fmt.Printf("(%d/%d) Loading siad...\n", i, len(config.Siad.Modules))
	srv, err := newSiadServer(config.Siad.APIaddr)
	if err != nil {
		return err
	}

	serverrs := make(chan error)
	go func() {
		serverrs <- srv.Serve()
	}()

	// Create all of the modules.
	var g modules.Gateway
	if strings.Contains(config.Siad.Modules, "g") {
		i++
		fmt.Printf("(%d/%d) Loading gateway...\n", i, len(config.Siad.Modules))
		g, err = gateway.New(config.Siad.RPCaddr, filepath.Join(config.Siad.SiaDir, modules.GatewayDir))
		if err != nil {
			return err
		}
	}
	var cs modules.ConsensusSet
	if strings.Contains(config.Siad.Modules, "c") {
		i++
		fmt.Printf("(%d/%d) Loading consensus...\n", i, len(config.Siad.Modules))
		cs, err = consensus.New(g, filepath.Join(config.Siad.SiaDir, modules.ConsensusDir))
		if err != nil {
			return err
		}
	}
	var e modules.Explorer
	if strings.Contains(config.Siad.Modules, "e") {
		i++
		fmt.Printf("(%d/%d) Loading explorer...\n", i, len(config.Siad.Modules))
		e, err = explorer.New(cs, filepath.Join(config.Siad.SiaDir, modules.ExplorerDir))
		if err != nil {
			return err
		}
	}
	var tpool modules.TransactionPool
	if strings.Contains(config.Siad.Modules, "t") {
		i++
		fmt.Printf("(%d/%d) Loading transaction pool...\n", i, len(config.Siad.Modules))
		tpool, err = transactionpool.New(cs, g, filepath.Join(config.Siad.SiaDir, modules.TransactionPoolDir))
		if err != nil {
			return err
		}
	}
	var w modules.Wallet
	if strings.Contains(config.Siad.Modules, "w") {
		i++
		fmt.Printf("(%d/%d) Loading wallet...\n", i, len(config.Siad.Modules))
		w, err = wallet.New(cs, tpool, filepath.Join(config.Siad.SiaDir, modules.WalletDir))
		if err != nil {
			return err
		}
	}
	var m modules.Miner
	if strings.Contains(config.Siad.Modules, "m") {
		i++
		fmt.Printf("(%d/%d) Loading miner...\n", i, len(config.Siad.Modules))
		m, err = miner.New(cs, tpool, w, filepath.Join(config.Siad.SiaDir, modules.MinerDir))
		if err != nil {
			return err
		}
	}
	var h modules.Host
	if strings.Contains(config.Siad.Modules, "h") {
		i++
		fmt.Printf("(%d/%d) Loading host...\n", i, len(config.Siad.Modules))
		h, err = host.New(cs, tpool, w, config.Siad.HostAddr, filepath.Join(config.Siad.SiaDir, modules.HostDir))
		if err != nil {
			return err
		}
	}
	var r modules.Renter
	if strings.Contains(config.Siad.Modules, "r") {
		i++
		fmt.Printf("(%d/%d) Loading renter...\n", i, len(config.Siad.Modules))
		r, err = renter.New(cs, w, tpool, filepath.Join(config.Siad.SiaDir, modules.RenterDir))
		if err != nil {
			return err
		}
	}

	// Create the Sia API 
	a := api.NewAPI(
		config.Siad.RequiredUserAgent,
		config.APIPassword,
		cs,
		e,
		g,
		h,
		m,
		r,
		tpool,
		w,
	)

	// connect the API to the server
	srv.ConnectAPI(a)

	// stop the server if a kill signal is caught
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill)
	defer signal.Stop(sigChan)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-sigChan:
			fmt.Println("\rCaught stop signal, quitting...")
			srv.Close()
		case <-stop:
			// Don't leave a dangling goroutine.
		}
	}()

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

	// Wait until the Server stops serving to exit
	err = <-serverrs
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

	// Start siad. startDaemon will only return when it is shutting down.
	err := startDaemon(globalConfig)
	if err != nil {
		die(err)
	}
}
