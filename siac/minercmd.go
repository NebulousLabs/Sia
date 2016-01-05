package main

import (
	"fmt"

	"github.com/NebulousLabs/Sia/api"

	"github.com/spf13/cobra"
)

var (
	minerCmd = &cobra.Command{
		Use:   "miner",
		Short: "Perform miner actions",
		Long:  "Interact with the miner",
		Run:   wrap(minerstatuscmd),
	}

	minerStartCmd = &cobra.Command{
		Use:   "start",
		Short: "Start cpu mining",
		Long:  "Start cpu mining, if the miner is already running, this command does nothing",
		Run:   wrap(minerstartcmd),
	}

	minerStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "View miner status",
		Long:  "View the current mining status",
		Run:   wrap(minerstatuscmd),
	}

	minerStopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop mining",
		Long:  "Stop mining (this may take a few moments).",
		Run:   wrap(minerstopcmd),
	}
)

func minerstartcmd() {
	err := get("/miner/start")
	if err != nil {
		fmt.Println("Could not start miner:", err)
		return
	}
	fmt.Println("CPU Miner is now running.")
}

func minerstatuscmd() {
	status := new(api.MinerGET)
	err := getAPI("/miner", status)
	if err != nil {
		fmt.Println("Could not get miner status:", err)
		return
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

func minerstopcmd() {
	err := get("/miner/stop")
	if err != nil {
		fmt.Println("Could not stop miner:", err)
		return
	}
	fmt.Println("Stopped mining.")
}
