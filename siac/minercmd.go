package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/modules"
)

var (
	minerCmd = &cobra.Command{
		Use:   "miner",
		Short: "Perform miner actions",
		Long:  "Start mining, stop mining, or view the current mining status, including number of threads, deposit address, and more.",
		Run:   wrap(minerstatuscmd),
	}

	minerStartCmd = &cobra.Command{
		Use:   "start [threads]",
		Short: "Start mining on 'threads' threads",
		Long:  "Start mining on a specified number of threads. If the miner is already running, the number of threads is adjusted.",
		Run:   wrap(minerstartcmd),
	}

	minerStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "View miner status",
		Long:  "View the current mining status, including number of threads, deposit address, and more.",
		Run:   wrap(minerstatuscmd),
	}

	minerStopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop mining",
		Long:  "Stop mining (this may take a few moments).",
		Run:   wrap(minerstopcmd),
	}
)

func minerstartcmd(threads string) {
	err := post("/miner/start", "threads="+threads)
	if err != nil {
		fmt.Println("Could not start miner:", err)
		return
	}
	fmt.Println("Now mining on " + threads + " threads.")
}

func minerstatuscmd() {
	status := new(modules.MinerInfo)
	err := getAPI("/miner/status", status)
	if err != nil {
		fmt.Println("Could not get miner status:", err)
		return
	}
	fmt.Printf(`Miner status:
State:   %s
Threads: %d (%d active)
Address: %x
`, status.State, status.Threads, status.RunningThreads, status.Address)
}

func minerstopcmd() {
	err := post("/miner/stop", "")
	if err != nil {
		fmt.Println("Could not stop miner:", err)
		return
	}
	fmt.Println("Stopped mining.")
}
