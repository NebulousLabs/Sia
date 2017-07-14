package main

import (
	"fmt"

	"github.com/NebulousLabs/Sia/api"

	"github.com/spf13/cobra"
)

var (
	poolCmd = &cobra.Command{
		Use:   "pool",
		Short: "Perform pool actions",
		Long:  "Perform pool actions and view pool status.",
		Run:   wrap(poolcmd),
	}

	poolStartCmd = &cobra.Command{
		Use:   "start",
		Short: "Start mining pool",
		Long:  "Start mining pool, if the pool is already running, this command does nothing",
		Run:   wrap(poolstartcmd),
	}

	poolStopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop mining pool",
		Long:  "Stop mining pool (this may take a few moments).",
		Run:   wrap(poolstopcmd),
	}
)

// poolstartcmd is the handler for the command `siac pool start`.
// Starts the mining pool.
func poolstartcmd() {
	err := get("/pool/start")
	if err != nil {
		die("Could not start mining pool:", err)
	}
	fmt.Println("Mining pool is now running.")
}

// poolcmd is the handler for the command `siac pool`.
// Prints the status of the pool.
func poolcmd() {
	status := new(api.PoolGET)
	err := getAPI("/pool", status)
	if err != nil {
		die("Could not get pool status:", err)
	}

	poolStr := "off"
	if status.PoolRunning {
		poolStr = "on"
	}
	fmt.Printf(`Pool status:
Mining Pool:   %s
Pool Hashrate: %v GH/s
Blocks Mined: %d
`, poolStr, status.PoolHashrate/1000000000, status.BlocksMined)
}

// poolstopcmd is the handler for the command `siac pool stop`.
// Stops the CPU miner.
func poolstopcmd() {
	err := get("/pool/stop")
	if err != nil {
		die("Could not stop pool:", err)
	}
	fmt.Println("Stopped mining pool.")
}
