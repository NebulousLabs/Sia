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
		Run:   wrap(gatewaycmd),
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

	gatewayAddressCmd = &cobra.Command{
		Use:   "address",
		Short: "Print the gateway address",
		Long:  "Print the network address of the gateway.",
		Run:   wrap(gatewayaddresscmd),
	}

	gatewayListCmd = &cobra.Command{
		Use:   "list",
		Short: "View a list of peers",
		Long:  "View the current peer list.",
		Run:   wrap(gatewaylistcmd),
	}
)

// gatewayaddcmd is the handler for the command `siac gateway add [address]`.
// Adds a new peer to the peer list.
func gatewayaddcmd(addr string) {
	err := post("/gateway/add/"+addr, "")
	if err != nil {
		die("Could not add peer", err)
	}
	fmt.Println("Added", addr, "to peer list.")
}

// gatewayremovecmd is the handler for the command `siac gateway remove [address]`.
// Removes a peer from the peer list.
func gatewayremovecmd(addr string) {
	err := post("/gateway/remove/"+addr, "")
	if err != nil {
		die("Could not remove peer", err)
	}
	fmt.Println("Removed", addr, "from peer list.")
}

// gatewayaddresscmd is the handler for the command `siac gateway address`.
// Prints the gateway's network address.
func gatewayaddresscmd() {
	var info api.GatewayInfo
	err := getAPI("/gateway", &info)
	if err != nil {
		die("Could not get gateway address", err)
	}
	fmt.Println("Address:", info.NetAddress)
}

// gatewaycmd is the handler for the command `siac gateway`.
// Prints the gateway's network address and number of peers.
func gatewaycmd() {
	var info api.GatewayInfo
	err := getAPI("/gateway", &info)
	if err != nil {
		die("Could not get gateway address", err)
	}
	fmt.Println("Address:", info.NetAddress)
	fmt.Println("Active peers:", len(info.Peers))
}

// gatewaylistcmd is the handler for the command `siac gateway list`.
// Prints a list of all peers.
func gatewaylistcmd() {
	var info api.GatewayInfo
	err := getAPI("/gateway", &info)
	if err != nil {
		die("Could not get peer list", err)
	}
	if len(info.Peers) == 0 {
		fmt.Println("No peers to show.")
		return
	}
	fmt.Println(len(info.Peers), "active peers:")
	for _, peer := range info.Peers {
		fmt.Println("\t", peer)
	}
}
