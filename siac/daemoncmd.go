package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the Sia daemon",
		Long:  "Stop the Sia daemon.",
		Run:   wrap(stopcmd),
	}
)

// stopcmd is the handler for the command `siac stop`.
// Stops the daemon.
func stopcmd() {
	err := get("/daemon/stop")
	if err != nil {
		die("Could not stop daemon:", err)
	}
	fmt.Println("Sia daemon stopped.")
}
