package main

import (
	"fmt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/spf13/cobra"
)

var (
	daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Change daemon settings",
		Long:  "View or modify daemon settings.",
		Run:   wrap(daemoncmd),
	}

	memloggingCmd = &cobra.Command{
		Use:   "memlogging",
		Short: "Check or set memlogging setting",
		Long:  "Pass the values 'true', or 'false' to change the memlogging setting. No params gives the current setting.",
		Run:   memloggingcmd,
	}

	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the Sia daemon",
		Long:  "Stop the Sia daemon.",
		Run:   wrap(stopcmd),
	}

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

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information.",
		Run:   wrap(versioncmd),
	}
)

type updateInfo struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
}

type daemonVersion struct {
	Version string
}

type memloggingInfo struct {
	Active bool `json:"active"`
}

func daemoncmd() {
	fmt.Printf("Try the command 'daemon memlogging'")
}

func memloggingcmd(cmd *cobra.Command, args []string) {
	switch len(args) {
	case 0:
		var loggingInfo memloggingInfo
		err := getAPI("/daemon/memlogging", &loggingInfo)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println("Memlogging Active: ", loggingInfo.Active)

	case 1:
		err := post("/daemon/memlogging", "active="+args[0])
		if err != nil {
			die("Could not update memlogging settings:", err)
		}
	default:
		fmt.Println("Error: expected 0 or 1 params to this command")
	}
}

// version prints the version of siac and siad.
func versioncmd() {
	fmt.Println("Sia Client v" + build.Version)
	var versioninfo daemonVersion
	err := getAPI("/daemon/version", &versioninfo)
	if err != nil {
		fmt.Println("Could not get daemon version:", err)
		return
	}
	fmt.Println("Sia Daemon v" + versioninfo.Version)
}

// stopcmd is the handler for the command `siac stop`.
// Stops the daemon.
func stopcmd() {
	err := get("/daemon/stop")
	if err != nil {
		die("Could not stop daemon:", err)
	}
	fmt.Println("Sia daemon stopped.")
}

func updatecmd() {
	var update updateInfo
	err := getAPI("/daemon/update", &update)
	if err != nil {
		fmt.Println("Could not check for update:", err)
		return
	}
	if !update.Available {
		fmt.Println("Already up to date.")
		return
	}

	err = post("/daemon/update", "")
	if err != nil {
		fmt.Println("Could not apply update:", err)
		return
	}
	fmt.Printf("Updated to version %s! Restart siad now.\n", update.Version)
}

func updatecheckcmd() {
	var update updateInfo
	err := getAPI("/daemon/update", &update)
	if err != nil {
		fmt.Println("Could not check for update:", err)
		return
	}
	if update.Available {
		fmt.Printf("A new release (v%s) is available! Run 'siac update' to install it.\n", update.Version)
	} else {
		fmt.Println("Up to date.")
	}
}
