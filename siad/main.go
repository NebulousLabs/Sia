package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/build"
)

var (
	// globalConfig is used by the cobra package to fill out the configuration
	// variables.
	globalConfig Config
)

// exit codes
// inspired by sysexits.h
const (
	exitCodeGeneral = 1  // Not in sysexits.h, but is standard practice.
	exitCodeUsage   = 64 // EX_USAGE in sysexits.h
)

// The Config struct contains all configurable variables for siad. It is
// compatible with gcfg.
type Config struct {
	Siad struct {
		APIaddr  string
		RPCaddr  string
		HostAddr string

		Modules           string
		NoBootstrap       bool
		RequiredUserAgent string

		Profile    bool
		ProfileDir string
		SiaDir     string
	}
}

// die prints its arguments to stderr, then exits the program with the default
// error code.
func die(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(exitCodeGeneral)
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
	root.PersistentFlags().StringVarP(&globalConfig.Siad.RequiredUserAgent, "agent", "A", "Sia-Agent", "required substring for the user agent")
	root.PersistentFlags().StringVarP(&globalConfig.Siad.HostAddr, "host-addr", "H", ":9982", "which port the host listens on")
	root.PersistentFlags().StringVarP(&globalConfig.Siad.ProfileDir, "profile-directory", "P", "profiles", "location of the profiling directory")
	root.PersistentFlags().StringVarP(&globalConfig.Siad.APIaddr, "api-addr", "a", "localhost:9980", "which host:port the API server listens on")
	root.PersistentFlags().StringVarP(&globalConfig.Siad.SiaDir, "sia-directory", "d", "", "location of the sia directory")
	root.PersistentFlags().BoolVarP(&globalConfig.Siad.NoBootstrap, "no-bootstrap", "n", false, "disable bootstrapping on this run")
	root.PersistentFlags().BoolVarP(&globalConfig.Siad.Profile, "profile", "p", false, "enable profiling")
	root.PersistentFlags().StringVarP(&globalConfig.Siad.RPCaddr, "rpc-addr", "r", ":9981", "which port the gateway listens on")
	root.PersistentFlags().StringVarP(&globalConfig.Siad.Modules, "modules", "M", "cghmrtw", "enabled modules")

	// Parse cmdline flags, overwriting both the default values and the config
	// file values.
	if err := root.Execute(); err != nil {
		// Since no commands return errors (all commands set Command.Run instead of
		// Command.RunE), Command.Execute() should only return an error on an
		// invalid command or flag. Therefore Command.Usage() was called (assuming
		// Command.SilenceUsage is false) and we should exit with exitCodeUsage.
		os.Exit(exitCodeUsage)
	}
}
