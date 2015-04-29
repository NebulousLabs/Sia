package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

var (
	// A global config variable is needed to work with cobra's flag system.
	config Config

	// started is a channel that's used during testing to inform the test suite
	// that initialization of the daemon has completed.
	started chan struct{}
)

// The Config struct contains all configurable variables for siad. It is
// compatible with gcfg.
type Config struct {
	Siad struct {
		NoBootstrap bool

		APIaddr  string
		RPCaddr  string
		HostAddr string

		SiaDir string
	}
}

// avail checks if a file is available from the disk.
func avail(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// init
func init() {
	// Set the default config values.
	config.Siad.NoBootstrap = false
	config.Siad.APIaddr = "localhost:9980"
	config.Siad.RPCaddr = ":9981"
	config.Siad.HostAddr = ":9982"

	// Initialize the started channel, only used for testing.
	started = make(chan struct{})

}

// versionCmd is a cobra command that prints the version of siad.
func versionCmd(*cobra.Command, []string) {
	fmt.Println("Sia Daemon v" + api.VERSION)
}

// main establishes a set of commands and flags using the cobra package.
func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Daemon v" + api.VERSION,
		Long:  "Sia Daemon v" + api.VERSION,
		Run:   startDaemonCmd,
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information about the Sia Daemon",
		Run:   versionCmd,
	})

	// Set default values, which have the lowest priority.
	root.PersistentFlags().BoolVarP(&config.Siad.NoBootstrap, "no-bootstrap", "n", config.Siad.NoBootstrap, "disable bootstrapping on this run")
	root.PersistentFlags().StringVarP(&config.Siad.APIaddr, "api-addr", "a", config.Siad.APIaddr, "which host:port the API server listens on")
	root.PersistentFlags().StringVarP(&config.Siad.RPCaddr, "rpc-addr", "r", config.Siad.RPCaddr, "which port the gateway listens on")
	root.PersistentFlags().StringVarP(&config.Siad.HostAddr, "host-addr", "H", config.Siad.HostAddr, "which port the host listens on")
	root.PersistentFlags().StringVarP(&config.Siad.SiaDir, "sia-directory", "d", config.Siad.SiaDir, "location of the sia directory")

	// Parse cmdline flags, overwriting both the default values and the config
	// file values.
	root.Execute()
}
