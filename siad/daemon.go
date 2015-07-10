package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

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
func startDaemon() error {
	// Establish multithreading.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Print a startup message.
	fmt.Println("siad is loading")
	loadStart := time.Now().UnixNano()

	// Establish cpu profiling. The current implementation only profiles
	// loading the blockchain into memory.
	if config.Siad.Profile {
		cpuProfileFile, err := os.Create(filepath.Join(config.Siad.ProfileDir, "startup-cpu-profile.prof"))
		if err != nil {
			return err
		}
		pprof.StartCPUProfile(cpuProfileFile)
	}

	// Create all of the modules.
	gateway, err := gateway.New(config.Siad.RPCaddr, filepath.Join(config.Siad.SiaDir, modules.GatewayDir))
	if err != nil {
		return err
	}
	state, err := consensus.New(gateway, filepath.Join(config.Siad.SiaDir, modules.ConsensusDir))
	if err != nil {
		return err
	}
	tpool, err := transactionpool.New(state, gateway)
	if err != nil {
		return err
	}
	wallet, err := wallet.New(state, tpool, filepath.Join(config.Siad.SiaDir, modules.WalletDir))
	if err != nil {
		return err
	}
	miner, err := miner.New(state, tpool, wallet, filepath.Join(config.Siad.SiaDir, modules.MinerDir))
	if err != nil {
		return err
	}
	hostdb, err := hostdb.New(state, gateway)
	if err != nil {
		return err
	}
	host, err := host.New(state, hostdb, tpool, wallet, config.Siad.HostAddr, filepath.Join(config.Siad.SiaDir, modules.HostDir))
	if err != nil {
		return err
	}
	renter, err := renter.New(state, hostdb, wallet, filepath.Join(config.Siad.SiaDir, modules.RenterDir))
	if err != nil {
		return err
	}
	srv, err := api.NewServer(config.Siad.APIaddr, state, gateway, host, hostdb, miner, renter, tpool, wallet, nil)
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

	// Stop the cpu profiler now that the initial blockchain loading is
	// complete.
	if config.Siad.Profile {
		pprof.StopCPUProfile()
	}

	// Print a 'startup complete' message.
	startupTime := time.Now().UnixNano() - loadStart
	fmt.Println("siad has finished loading after", float64(startupTime)/1e9, "seconds")

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
	if config.Siad.Profile {
		err := os.MkdirAll(config.Siad.ProfileDir, 0700)
		if err != nil {
			fmt.Println(err)
			return
		}

		// Create a goroutime to log the number of gothreads in use every 30
		// seconds.
		go func() {
			// Create a logger for the goroutine.
			logFile, err := os.OpenFile(filepath.Join(config.Siad.ProfileDir, "goroutineCount.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
			if err != nil {
				fmt.Println("Goroutine logging failed:", err)
				return
			}
			log := log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
			log.Println("Goroutine logger started. The number of goroutines in use will be printed every 30 seoncds.")

			// Inifinite loop to print out the goroutine count.
			for {
				log.Println(runtime.NumGoroutine())
				time.Sleep(time.Second * 30)
			}
		}()

		// Create a goroutine to update the memory profile.
		go func() {
			memFile, err := os.Create(filepath.Join(config.Siad.ProfileDir, "memprofile.prof"))
			if err != nil {
				fmt.Println("Memory profiling failed:", err)
				return
			}

			// Infinite loop to update the memory profile.
			for {
				pprof.WriteHeapProfile(memFile)
				time.Sleep(time.Minute)
			}
		}()
	}

	// Start siad. startDaemon will only return when it is shutting down.
	err := startDaemon()
	if err != nil {
		fmt.Println(err)
	}
}
