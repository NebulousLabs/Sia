package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/sia"
)

var (
	updateCmd = &cobra.Command{
		Use:   "update [check|apply]",
		Short: "Update Sia",
		Long:  "Check for (and/or download) available updates for Sia.",
		Run:   wrap(updatecmd),
	}

	updateCheckCmd = &cobra.Command{
		Use:   "update check",
		Short: "Check for available updates",
		Long:  "Check for available updates.",
		Run:   wrap(updatecheckcmd),
	}

	updateApplyCmd = &cobra.Command{
		Use:   "update apply [version]",
		Short: "Apply an update",
		Long:  "Update Sia to 'version'. To use the latest version, run 'update apply current', or simply 'update'.",
		Run:   wrap(updateapplycmd),
	}

	statusCmd = &cobra.Command{
		Use:   "status",
		Short: "Print the current state of the daemon",
		Long:  "Query the daemon for values such as the current difficulty, target, height, peers, transactions, etc.",
		Run:   wrap(statuscmd),
	}

	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the Sia daemon",
		Long:  "Stop the Sia daemon.",
		Run:   wrap(stopcmd),
	}

	syncCmd = &cobra.Command{
		Use:   "sync",
		Short: "Synchronize with the network",
		Long:  "Attempt to synchronize with a randomly selected peer.",
		Run:   wrap(synccmd),
	}
)

// TODO: this should be defined outside of siac
type updateResp struct {
	Available bool
	Version   string
}

func updatecmd() {
	update := new(updateResp)
	err := getAPI("/update/check", update)
	if err != nil {
		fmt.Println("Could not check for update:", err)
		return
	}
	if !update.Available {
		fmt.Println("Already up to date.")
		return
	}
	err = callAPI("/update/apply?version=" + update.Version)
	if err != nil {
		fmt.Println("Could not apply update:", err)
		return
	}
	fmt.Printf("Updated to version %s! Restart siad now.\n", update.Version)
}

func updatecheckcmd() {
	update := new(updateResp)
	err := getAPI("/update/check", update)
	if err != nil {
		fmt.Println("Could not check for update:", err)
		return
	}
	if !update.Available {
		fmt.Println("Up to date!")
		return
	}
	fmt.Printf("Update %s is available! Run 'siac update apply %s' to install it.\n", update.Version, update.Version)
}

func updateapplycmd(version string) {
	err := callAPI("/update/apply?version=" + version)
	if err != nil {
		fmt.Println("Could not apply update:", err)
		return
	}
	fmt.Printf("Updated to version %s! Restart siad now.\n", version)
}

func statuscmd() {
	status := new(sia.StateInfo)
	err := getAPI("/status", status)
	if err != nil {
		fmt.Println("Could not get daemon status:", err)
		return
	}
	fmt.Printf(`Block:  %v
Height: %v
Target: %v
Depth:  %v
`, status.CurrentBlock, status.Height, status.Target, status.Depth)
}

func stopcmd() {
	err := callAPI("/stop")
	if err != nil {
		fmt.Println("Could not stop daemon:", err)
		return
	}
	fmt.Println("Sia daemon stopped.")
}

func synccmd() {
	err := callAPI("/sync")
	if err != nil {
		fmt.Println("Could not sync:", err)
	}
	fmt.Println("Sync initiated.")
}
