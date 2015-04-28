package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/modules"
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
	err := callAPI("/consensus/synchronize")
	if err != nil {
		fmt.Println("Could not synchronize:", err)
		return
	}
	fmt.Println("Sync initiated.")
}

func consensusstatuscmd() {
	var info struct {
		Address modules.NetAddress
		Peers   []modules.NetAddress
	}
	err := getAPI("/consensus/status", &info)
	if err != nil {
		fmt.Println("Could not get consensus status:", err)
		return
	}
	fmt.Println("Address:", info.Address)
	if len(info.Peers) == 0 {
		fmt.Println("No peers to show.")
		return
	}
	fmt.Println(len(info.Peers), "active peers:")
	for _, peer := range info.Peers {
		fmt.Println("\t", peer)
	}
}
