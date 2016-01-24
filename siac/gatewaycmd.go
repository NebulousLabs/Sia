package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

var (
	gatewayCmd = &cobra.Command{
		Use:   "gateway",
		Short: "Perform gateway actions",
		Long:  "Add or remove a peer, view the current peer list, or synchronize to the network.",
		Run:   wrap(gatewaylistcmd),
	}

	gatewayAddCmd = &cobra.Command{
		Use:   "add [address]",
		Short: "Add a peer",
		Long:  "Add a new peer to the peer list.",
		Run:   wrap(gatewayaddcmd),
	}

	gatewayRemoveCmd = &cobra.Command{
		Use:   "remove [address]",
		Short: "Remove a peer",
		Long:  "Remove a peer from the peer list.",
		Run:   wrap(gatewayremovecmd),
	}

	gatewayListCmd = &cobra.Command{
		Use:   "list",
		Short: "View a list of peers",
		Long:  "View the current peer list.",
		Run:   wrap(gatewaylistcmd),
	}
)

func gatewayaddcmd(addr string) {
	err := post("/gateway/add/"+addr, "")
	if err != nil {
		fmt.Println("Could not add peer:", err)
		return
	}
	fmt.Println("Added", addr, "to peer list.")
}

func gatewayremovecmd(addr string) {
	err := post("/gateway/remove/"+addr, "")
	if err != nil {
		fmt.Println("Could not remove peer:", err)
		return
	}
	fmt.Println("Removed", addr, "from peer list.")
}

func gatewaylistcmd() {
	var info api.GatewayInfo
	err := getAPI("/gateway", &info)
	if err != nil {
		fmt.Println("Could not get peer list:", err)
		return
	}
	fmt.Println("Address:", info.NetAddress)
	if len(info.Peers) == 0 {
		fmt.Println("No peers to show.")
		return
	}
	fmt.Println(len(info.Peers), "active peers:")
	for _, peer := range info.Peers {
		fmt.Println("\t", peer)
	}
}
