package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var apiPort uint16
var rpcPort uint16

// Calls CreateEnvironment(), which will handle everything else.
func startEnvironment(cmd *cobra.Command, args []string) {
	_, err := CreateEnvironment(rpcPort, apiPort, true)
	if err != nil {
		println(err.Error())
		return
	}
}

// Prints version information about Sia Daemon.
func version(cmd *cobra.Command, args []string) {
	fmt.Println("Sia Daemon v0.1.0")
}

func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Daemon v0.1.0",
		Long:  "Sia Daemon v0.1.0",
		Run:   startEnvironment,
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information about the Sia Daemon",
		Run:   version,
	})

	// Add flags for the api port and rpc port.
	root.Flags().Uint16VarP(&apiPort, "api-port", "a", 9980, "Which port is used to communicate with the user.")
	root.Flags().Uint16VarP(&rpcPort, "rpc-port", "r", 9988, "Which port is used when talking to other nodes on the network.")

	// Start the party.
	root.Execute()
}
