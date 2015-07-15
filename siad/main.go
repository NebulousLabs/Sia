package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/build"
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

		Profile    bool
		ProfileDir string
	}
}

// init creates the channel that's used to communicate with the testing
// framework.
func init() {
	// Initialize the started channel, only used for testing.
	started = make(chan struct{}, 1)
}

// versionCmd is a cobra command that prints the version of siad.
func versionCmd(*cobra.Command, []string) {
	fmt.Println("Sia Daemon v" + build.Version)
}

// main establishes a set of commands and flags using the cobra package.
func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Daemon v" + build.Version,
		Long:  "Sia Daemon v" + build.Version,
		Run:   startDaemonCmd,
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information about the Sia Daemon",
		Run:   versionCmd,
	})

	// Set default values, which have the lowest priority.
	root.PersistentFlags().BoolVarP(&config.Siad.NoBootstrap, "no-bootstrap", "n", false, "disable bootstrapping on this run")
	root.PersistentFlags().StringVarP(&config.Siad.APIaddr, "api-addr", "a", "localhost:9980", "which host:port the API server listens on")
	root.PersistentFlags().StringVarP(&config.Siad.RPCaddr, "rpc-addr", "r", ":9981", "which port the gateway listens on")
	root.PersistentFlags().StringVarP(&config.Siad.HostAddr, "host-addr", "H", ":9982", "which port the host listens on")
	root.PersistentFlags().StringVarP(&config.Siad.SiaDir, "sia-directory", "d", "", "location of the sia directory")
	root.PersistentFlags().BoolVarP(&config.Siad.Profile, "profile", "p", false, "enable profiling")
	root.PersistentFlags().StringVarP(&config.Siad.ProfileDir, "profile-directory", "P", "profiles", "location of the profiling directory")

	// Parse cmdline flags, overwriting both the default values and the config
	// file values.
	root.Execute()
}
