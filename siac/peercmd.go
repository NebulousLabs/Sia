package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/network"
)

var (
	peerCmd = &cobra.Command{
		Use:   "peer [add|remove|status]",
		Short: "Perform peer actions",
		Long:  "Add or remove a peer, or view the current peer list.",
		Run:   wrap(peerstatuscmd),
	}

	peerAddCmd = &cobra.Command{
		Use:   "peer add [addr]",
		Short: "Add a peer",
		Long:  "Add a new peer. The peer will only be added if it responds to a ping request.",
		Run:   wrap(peeraddcmd),
	}

	peerRemoveCmd = &cobra.Command{
		Use:   "peer remove [addr]",
		Short: "Remove a peer",
		Long:  "Remove a peer from the peer list.",
		Run:   wrap(peerremovecmd),
	}

	peerStatusCmd = &cobra.Command{
		Use:   "peer status",
		Short: "View a list of peers",
		Long:  "View the current peer list.",
		Run:   wrap(peerstatuscmd),
	}
)

func peeraddcmd(addr string) {
	err := callAPI("/peer/add?addr=" + addr)
	if err != nil {
		fmt.Println("Could not add peer:", err)
		return
	}
	fmt.Println("Added", addr, "to peer list.")
}

func peerremovecmd(addr string) {
	err := callAPI("/peer/remove?addr=" + addr)
	if err != nil {
		fmt.Println("Could not remove peer:", err)
		return
	}
	fmt.Println("Removed", addr, "from peer list.")
}

func peerstatuscmd() {
	var peers []network.Address
	err := getAPI("/peer/status", &peers)
	if err != nil {
		fmt.Println("Could not get peer status:", err)
		return
	}
	fmt.Println(len(peers), "active peers:")
	for _, peer := range peers {
		fmt.Println("\t", peer)
	}
}
