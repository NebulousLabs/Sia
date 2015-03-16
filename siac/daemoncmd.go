package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/consensus"
)

var (
	updateCmd = &cobra.Command{
		Use:   "update",
		Short: "Update Sia",
		Long:  "Check for (and/or download) available updates for Sia.",
		Run:   wrap(updatecmd),
	}

	updateCheckCmd = &cobra.Command{
		Use:   "check",
		Short: "Check for available updates",
		Long:  "Check for available updates.",
		Run:   wrap(updatecheckcmd),
	}

	updateApplyCmd = &cobra.Command{
		Use:   "apply [version]",
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
)

// TODO: this should be defined outside of siac
type updateResp struct {
	Available bool
	Version   string
}

func updatecmd() {
	update := new(updateResp)
	err := getAPI("/daemon/update/check", update)
	if err != nil {
		fmt.Println("Could not check for update:", err)
		return
	}
	if !update.Available {
		fmt.Println("Already up to date.")
		return
	}
	err = callAPI("/daemon/update/apply?version=" + update.Version)
	if err != nil {
		fmt.Println("Could not apply update:", err)
		return
	}
	fmt.Printf("Updated to version %s! Restart siad now.\n", update.Version)
}

func updatecheckcmd() {
	update := new(updateResp)
	err := getAPI("/daemon/update/check", update)
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
	err := callAPI("/daemon/update/apply?version=" + version)
	if err != nil {
		fmt.Println("Could not apply update:", err)
		return
	}
	fmt.Printf("Updated to version %s! Restart siad now.\n", version)
}

func statuscmd() {
	status := new(consensus.StateInfo)
	err := getAPI("/consensus/status", status)
	if err != nil {
		fmt.Println("Could not get daemon status:", err)
		return
	}
	fmt.Printf(`Block:  %v
Height: %v
Target: %v
`, status.CurrentBlock, status.Height, status.Target)
}

func stopcmd() {
	err := callAPI("/daemon/stop")
	if err != nil {
		fmt.Println("Could not stop daemon:", err)
		return
	}
	fmt.Println("Sia daemon stopped.")
}
