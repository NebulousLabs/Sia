package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"code.google.com/p/gcfg"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

var (
	config Config
	siaDir string
)

type Config struct {
	Siacore struct {
		RPCaddr     string
		NoBootstrap bool
	}

	Siad struct {
		APIaddr           string
		ConfigFilename    string
		DownloadDirectory string
	}
}

// Helper function for determining existence of a file. Technically, err != nil
// does not necessarily mean that the file does not exist, but it does mean
// that it cannot be read, and for our purposes these are equivalent.
func exists(filename string) bool {
	ex, err := homedir.Expand(filename)
	if err != nil {
		return false
	}
	_, err = os.Stat(ex)
	return err == nil
}

func init() {
	// locate siaDir by checking for config file
	switch {
	case exists("config"):
		siaDir = ""
	case exists("~/.config/sia/config"):
		siaDir = "~/.config/sia/"
	default:
		siaDir = ""
		fmt.Println("Warning: config file not found. Default values will be used.")
	}
}

func startEnvironment(*cobra.Command, []string) {
	// Set GOMAXPROCS equal to the number of cpu cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	daemonConfig := DaemonConfig{
		APIAddr: config.Siad.APIaddr,
		RPCAddr: config.Siacore.RPCaddr,

		SiaDir: siaDir,
	}
	d, err := newDaemon(daemonConfig)
	if err != nil {
		fmt.Println("Failed to create daemon:", err)
		return
	}

	// serve API requests
	err = d.srv.Serve()
	if err != nil {
		fmt.Println("API server quit unexpectedly:", err)
	}
}

func version(*cobra.Command, []string) {
	fmt.Println("Sia Daemon v" + api.VERSION)
}

func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Daemon v" + api.VERSION,
		Long:  "Sia Daemon v" + api.VERSION,
		Run:   startEnvironment,
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information about the Sia Daemon",
		Run:   version,
	})

	// Set default values, which have the lowest priority.
	defaultConfigFile := filepath.Join(siaDir, "config")
	root.PersistentFlags().StringVarP(&config.Siad.APIaddr, "api-addr", "a", "localhost:9980", "which host:port is used to communicate with the user")
	root.PersistentFlags().StringVarP(&config.Siacore.RPCaddr, "rpc-addr", "r", ":9988", "which port is used when talking to other nodes on the network")
	root.PersistentFlags().BoolVarP(&config.Siacore.NoBootstrap, "no-bootstrap", "n", false, "disable bootstrapping on this run")
	root.PersistentFlags().StringVarP(&config.Siad.ConfigFilename, "config-file", "c", defaultConfigFile, "location of the siad config file")

	// Create a Logger for this package
	logFile, err := os.OpenFile(filepath.Join(siaDir, "info.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("error opening log file: %v", err)
		os.Exit(1)
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime)

	// Load the config file, which will overwrite the default values.
	if exists(config.Siad.ConfigFilename) {
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
