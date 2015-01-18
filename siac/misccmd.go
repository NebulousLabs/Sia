package main

import (
	"fmt"

	"github.com/spf13/cobra"
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

func updatecmd() {
	version, err := getUpdate()
	if err != nil {
		fmt.Println("Could not update:", err)
		return
	}
	if version == VERSION {
		fmt.Println("Already up to date.")
		return
	}
	fmt.Printf("Updated to version %s! Restart siad now.\n", version)
}

func updatecheckcmd() {
	version, err := getUpdateCheck()
	if err != nil {
		fmt.Println("Could not check for update:", err)
		return
	}
	if version == VERSION {
		fmt.Println("Up to date!")
		return
	}
	fmt.Printf("Update %s is available! Run 'siac update %s' to install it.\n", version, version)
}

func updateapplycmd(version string) {
	err := getUpdateApply(version)
	if err != nil {
		fmt.Println("Could not apply update:", err)
		return
	}
	fmt.Println("Update", version, "applied! Restart siad now.")
}

func statuscmd() {
	status, err := getStatus()
	if err != nil {
		fmt.Println("Could not get daemon status:", err)
		return
	}
	fmt.Println(status)
}

func stopcmd() {
	err := getStop()
	if err != nil {
		fmt.Println("Could not stop daemon:", err)
		return
	}
	fmt.Println("Sia daemon stopped.")
}

func synccmd() {
	err := getSync()
	if err != nil {
		fmt.Println("Could not sync:", err)
	}
	fmt.Println("Sync initiated.")
}
