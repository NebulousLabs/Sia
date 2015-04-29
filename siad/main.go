package main

import (
	"fmt"
	"os"
	"path/filepath"

	"code.google.com/p/gcfg"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

var (
	// A global config variable is needed to work with cobra's flag system.
	config Config
)

// The Config struct contains all configurable variables for siad. It is
// compatible with gcfg.
type Config struct {
	Siad struct {
		NoBootstrap bool

		APIaddr  string
		RPCaddr  string
		HostAddr string

		ConfigFilename string
		SiaDir         string
	}
}

// avail checks if a file is available from the disk.
func avail(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// init looks for a config file.
func init() {
	homeConfig, err := homedir.Expand(filepath.Join("~", ".config", "sia", "config"))
	if err != nil {
		panic(err)
	}

	switch {
	case avail("config"):
		config.Siad.ConfigFilename = "config"
	case avail(homeConfig):
		config.Siad.ConfigFilename = homeConfig
	default:
	}
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
	root.PersistentFlags().StringVarP(&config.Siad.APIaddr, "api-addr", "a", "localhost:9980", "which host:port the API server listens on")
	root.PersistentFlags().StringVarP(&config.Siad.RPCaddr, "rpc-addr", "r", ":9988", "which port the gateway listens on")
	root.PersistentFlags().StringVarP(&config.Siad.HostAddr, "host-addr", "H", ":9990", "which port the host listens on")
	root.PersistentFlags().BoolVarP(&config.Siad.NoBootstrap, "no-bootstrap", "n", false, "disable bootstrapping on this run")
	root.PersistentFlags().StringVarP(&config.Siad.ConfigFilename, "config-file", "c", config.Siad.ConfigFilename, "location of the siad config file")
	root.PersistentFlags().StringVarP(&config.Siad.SiaDir, "sia-directory", "s", "", "location of the sia directory")

	// Load the config file, which will overwrite the default values.
	if avail(config.Siad.ConfigFilename) {
		configFilename, err := homedir.Expand(config.Siad.ConfigFilename)
		if err != nil {
			fmt.Println("Failed to load config file:", err)
			return
		}
		if err := gcfg.ReadFileInto(&config, configFilename); err != nil {
			fmt.Println("Failed to load config file:", err)
			return
		}
	}

	// Parse cmdline flags, overwriting both the default values and the config
	// file values.
	root.Execute()
}
