package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/modules"
)

var (
	gatewayCmd = &cobra.Command{
		Use:   "gateway",
		Short: "Perform gateway actions",
		Long:  "Add or remove a peer, view the current peer list, or synchronize to the network.",
		Run:   wrap(gatewaystatuscmd),
	}

	gatewayAddCmd = &cobra.Command{
		Use:   "add [addr]",
		Short: "Add a peer",
		Long:  "Add a new peer to the peer list.",
		Run:   wrap(gatewayaddcmd),
	}

	gatewayRemoveCmd = &cobra.Command{
		Use:   "remove [addr]",
		Short: "Remove a peer",
		Long:  "Remove a peer from the peer list.",
		Run:   wrap(gatewayremovecmd),
	}

	gatewaySynchronizeCmd = &cobra.Command{
		Use:   "sync",
		Short: "Synchronize with the network",
		Long:  "Attempt to synchronize with a randomly selected peer.",
		Run:   wrap(gatewaysynchronizecmd),
	}

	gatewayStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "View a list of peers",
		Long:  "View the current peer list.",
		Run:   wrap(gatewaystatuscmd),
	}
)

func gatewayaddcmd(addr string) {
	err := callAPI("/gateway/peer/add?addr=" + addr)
	if err != nil {
		fmt.Println("Could not add peer:", err)
		return
	}
	fmt.Println("Added", addr, "to peer list.")
}

func gatewayremovecmd(addr string) {
	err := callAPI("/gateway/peer/remove?addr=" + addr)
	if err != nil {
		fmt.Println("Could not remove peer:", err)
		return
	}
	fmt.Println("Removed", addr, "from peer list.")
}

func gatewaysynchronizecmd() {
	err := callAPI("/gateway/synchronize")
	if err != nil {
		fmt.Println("Could not synchronize:", err)
		return
	}
	fmt.Println("Sync initiated.")
}

func gatewaystatuscmd() {
	var info modules.GatewayInfo
	err := getAPI("/gateway/status", &info)
	if err != nil {
		fmt.Println("Could not get gateway status:", err)
		return
	}
	fmt.Println(len(info.Peers), "active peers:")
	for _, peer := range info.Peers {
		fmt.Println("\t", peer)
	}
}
