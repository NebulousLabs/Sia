package main

import (
	"fmt"
	"os"

	"github.com/NebulousLabs/Andromeda/siad"

	"github.com/spf13/cobra"
)

var port uint16

func walletStart(cmd *cobra.Command, args []string) {
	env, err := siad.CreateEnvironment(port)
	if err != nil {
		fmt.Println(err)
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
		Run:   func(_ *cobra.Command, _ []string) { fmt.Println("Sia Command Line Wallet v0.1.0") },
	})

	// Add a flag for setting the port.
	root.Flags().Uint16VarP(&port, "port", "p", 9988, "Which port siad uses to listen for network requests.")

	root.Execute()
}
