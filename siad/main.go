package main

import (
	"os"

	"github.com/spf13/cobra"
)

var apiPort uint16
var rpcPort uint16
var nobootstrap bool

// Calls CreateEnvironment(), which will handle everything else.
func startEnvironment(cmd *cobra.Command, args []string) {
	_, err := CreateEnvironment(rpcPort, apiPort, nobootstrap)
	if err != nil {
		println(err.Error())
		return
	}
}

// Prints version information about Sia Daemon.
func version(cmd *cobra.Command, args []string) {
	println("Sia Daemon v0.1.0")
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
	root.Flags().BoolVarP(&nobootstrap, "no-bootstrap", "n", false, "Disable bootstrapping on this run.")

	// Start the party.
	root.Execute()
}
