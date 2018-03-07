package main

import (
	"fmt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/node/api"
	"github.com/spf13/cobra"
)

var (
	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the Sia daemon",
		Long:  "Stop the Sia daemon.",
		Run:   wrap(stopcmd),
	}

	updateCheckCmd = &cobra.Command{
		Use:   "check",
		Short: "Check for available updates",
		Long:  "Check for available updates.",
		Run:   wrap(updatecheckcmd),
	}

	updateCmd = &cobra.Command{
		Use:   "update",
		Short: "Update Sia",
		Long:  "Check for (and/or download) available updates for Sia.",
		Run:   wrap(updatecmd),
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

// version prints the version of siac and siad.
func versioncmd() {
	fmt.Println("Sia Client")
	fmt.Println("\tVersion " + build.Version)
	if build.GitRevision != "" {
		fmt.Println("\tGit Revision " + build.GitRevision)
		fmt.Println("\tGit Branch " + build.GitBranch)
		fmt.Println("\tBuild Time " + build.BuildTime)
	}
	var dvg api.DaemonVersionGet
	err := getAPI("/daemon/version", &dvg)
	if err != nil {
		fmt.Println("Could not get daemon version:", err)
		return
	}
	fmt.Println("Sia Daemon")
	fmt.Println("\tVersion " + dvg.Version)
	if build.GitRevision != "" {
		fmt.Println("\tGit Revision " + dvg.GitRevision)
		fmt.Println("\tGit Branch " + dvg.GitBranch)
		fmt.Println("\tBuild Time " + dvg.BuildTime)
	}
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
