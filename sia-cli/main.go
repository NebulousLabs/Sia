package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func walletStart(cmd *cobra.Command, args []string) {
	env := createEnvironment()
	pollHome(env)
}

func main() {
	// Create the basic command.
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia CLI v0.1.0",
		Long:  "Sia Command Line Wallet, version 0.1.0",
		Run:   walletStart,
	}

	// Create a version command.
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information about the Sia Command Line Wallet.",
		Run:   func(_ *cobra.Command, _ []string) { fmt.Println("Sia Command Line Wallet v0.1.0") },
	})

	root.Execute()
}
