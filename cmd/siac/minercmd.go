package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	minerCmd = &cobra.Command{
		Use:   "miner",
		Short: "Perform miner actions",
		Long:  "Perform miner actions and view miner status.",
		Run:   wrap(minercmd),
	}

	minerStartCmd = &cobra.Command{
		Use:   "start",
		Short: "Start cpu mining",
		Long:  "Start cpu mining, if the miner is already running, this command does nothing",
		Run:   wrap(minerstartcmd),
	}

	minerStopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop mining",
		Long:  "Stop mining (this may take a few moments).",
		Run:   wrap(minerstopcmd),
	}
)

// minerstartcmd is the handler for the command `siac miner start`.
// Starts the CPU miner.
func minerstartcmd() {
	err := httpClient.MinerStartGet()
	if err != nil {
		die("Could not start miner:", err)
	}
	fmt.Println("CPU Miner is now running.")
}

// minercmd is the handler for the command `siac miner`.
// Prints the status of the miner.
func minercmd() {
	status, err := httpClient.MinerGet()
	if err != nil {
		die("Could not get miner status:", err)
	}

	miningStr := "off"
	if status.CPUMining {
		miningStr = "on"
	}
	fmt.Printf(`Miner status:
CPU Mining:   %s
CPU Hashrate: %v KH/s
Blocks Mined: %d (%d stale)
`, miningStr, status.CPUHashrate/1000, status.BlocksMined, status.StaleBlocksMined)
}

// minerstopcmd is the handler for the command `siac miner stop`.
// Stops the CPU miner.
func minerstopcmd() {
	err := httpClient.MinerStopGet()
	if err != nil {
		die("Could not stop miner:", err)
	}
	fmt.Println("Stopped mining.")
}
