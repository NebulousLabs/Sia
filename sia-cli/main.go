package main

import (
	"fmt"
	"os"

	"github.com/NebulousLabs/Andromeda/siad"

	"github.com/spf13/cobra"
)

var port uint16
var nobootstrap bool

func walletStart(cmd *cobra.Command, args []string) {
	env, err := siad.CreateEnvironment(port, nobootstrap)
	if err != nil {
		fmt.Println("Failed to initialize:", err)
		return
	}
	defer env.Close()

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
		Run:   func(*cobra.Command, []string) { fmt.Println("Sia Command Line Wallet v0.1.0") },
	})

	// Add a flag for setting the port.
	root.Flags().Uint16VarP(&port, "port", "p", 9988, "Which port siad uses to listen for network requests.")

	// Add a flag for setting the port.
	root.Flags().BoolVarP(&nobootstrap, "no-bootstrap", "n", false, "Don't attempt to bootstrap.")

	root.Execute()
}
