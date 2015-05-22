package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
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

	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the Sia daemon",
		Long:  "Stop the Sia daemon.",
		Run:   wrap(stopcmd),
	}
)

func updatecmd() {
	var update api.UpdateInfo
	err := getAPI("/daemon/updates/check", &update)
	if err != nil {
		fmt.Println("Could not check for update:", err)
		return
	}
	if !update.Available {
		fmt.Println("Already up to date.")
		return
	}
	err = get("/daemon/update/apply?version=" + update.Version)
	if err != nil {
		fmt.Println("Could not apply update:", err)
		return
	}
	fmt.Printf("Updated to version %s! Restart siad now.\n", update.Version)
}

func updatecheckcmd() {
	var update api.UpdateInfo
	err := getAPI("/daemon/updates/check", &update)
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
	err := post("/daemon/updates/apply", "version="+version)
	if err != nil {
		fmt.Println("Could not apply update:", err)
		return
	}
	fmt.Printf("Updated to version %s! Restart siad now.\n", version)
}

func stopcmd() {
	err := post("/daemon/stop", "")
	if err != nil {
		fmt.Println("Could not stop daemon:", err)
		return
	}
	fmt.Println("Sia daemon stopped.")
}
