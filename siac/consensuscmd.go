package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

var (
	consensusSynchronizeCmd = &cobra.Command{
		Use:   "sync",
		Short: "Synchronize with the network",
		Long:  "Attempt to synchronize with a randomly selected peer.",
		Run:   wrap(consensussynchronizecmd),
	}

	consensusStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Print the current state of the daemon",
		Long:  "Query the daemon for values such as the current difficulty, target, height, peers, transactions, etc.",
		Run:   wrap(consensusstatuscmd),
	}
)

func consensussynchronizecmd() {
	err := get("/consensus/synchronize")
	if err != nil {
		fmt.Println("Could not synchronize:", err)
		return
	}
	fmt.Println("Sync initiated.")
}

func consensusstatuscmd() {
	var info api.ConsensusInfo
	err := getAPI("/consensus/status", &info)
	if err != nil {
		fmt.Println("Could not get daemon status:", err)
		return
	}
	fmt.Printf(`Block:  %v
Height: %v
Target: %v
`, info.CurrentBlock, info.Height, info.Target)
}
