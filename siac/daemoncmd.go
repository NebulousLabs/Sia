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

func stopcmd() {
	err := get("/daemon/stop")
	if err != nil {
		fmt.Println("Could not stop daemon:", err)
		return
	}
	fmt.Println("Sia daemon stopped.")
}
