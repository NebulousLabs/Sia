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

	"github.com/NebulousLabs/Sia/modules"
)

var (
	config  Config
	homeDir string
	siaDir  string
)

type Config struct {
	Siacore struct {
		RPCaddr       string
		HostDirectory string
		NoBootstrap   bool
	}

	Siad struct {
		APIaddr           string
		ConfigFilename    string
		DownloadDirectory string
		WalletFile        string
	}
}

// expand all ~ characters in Config values
func (c *Config) expand() (err error) {
	c.Siacore.HostDirectory, err = homedir.Expand(c.Siacore.HostDirectory)
	if err != nil {
		return
	}
	c.Siad.APIaddr, err = homedir.Expand(c.Siad.APIaddr)
	if err != nil {
		return
	}
	c.Siad.ConfigFilename, err = homedir.Expand(c.Siad.ConfigFilename)
	if err != nil {
		return
	}
	c.Siad.DownloadDirectory, err = homedir.Expand(c.Siad.DownloadDirectory)
	if err != nil {
		return
	}
	c.Siad.WalletFile, err = homedir.Expand(c.Siad.WalletFile)
	if err != nil {
		return
	}

	return
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
		fmt.Println("Warning: config file not found. Default values will be used.")
	}
}

func startEnvironment(*cobra.Command, []string) {
	// Set GOMAXPROCS equal to the number of cpu cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	daemonConfig := DaemonConfig{
		APIAddr: config.Siad.APIaddr,
		RPCAddr: config.Siacore.RPCaddr,

		HostDir: config.Siacore.HostDirectory,

		Threads: 1,

		DownloadDir: config.Siad.DownloadDirectory,

		WalletDir: config.Siad.WalletFile,
	}
	err := config.expand()
	if err != nil {
		fmt.Println("Bad config value:", err)
		return
	}
	d, err := newDaemon(daemonConfig)
	if err != nil {
		fmt.Println("Failed to create daemon:", err)
		return
	}

	// join the network
	//
	// TODO: This should proabably happen in newDaemon.
	if !config.Siacore.NoBootstrap {
		go d.gateway.Bootstrap(modules.BootstrapPeers[0])
	}

	// listen for API requests
	err = d.listen()
	if err != nil {
		fmt.Println("API server quit unexpectedly:", err)
	}
}

func version(*cobra.Command, []string) {
	fmt.Println("Sia Daemon v" + VERSION)
}

func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Daemon v" + VERSION,
		Long:  "Sia Daemon v" + VERSION,
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
	defaultHostDir := filepath.Join(siaDir, "hostdir")
	defaultDownloadDir := "~/Downloads"
	defaultWalletFile := filepath.Join(siaDir, "sia.wallet")
	root.PersistentFlags().StringVarP(&config.Siad.APIaddr, "api-addr", "a", "localhost:9980", "which host:port is used to communicate with the user")
	root.PersistentFlags().StringVarP(&config.Siacore.RPCaddr, "rpc-addr", "r", ":9988", "which port is used when talking to other nodes on the network")
	root.PersistentFlags().BoolVarP(&config.Siacore.NoBootstrap, "no-bootstrap", "n", false, "disable bootstrapping on this run")
	root.PersistentFlags().StringVarP(&config.Siad.ConfigFilename, "config-file", "c", defaultConfigFile, "location of the siad config file")
	root.PersistentFlags().StringVarP(&config.Siacore.HostDirectory, "host-dir", "H", defaultHostDir, "location of hosted files")
	root.PersistentFlags().StringVarP(&config.Siad.DownloadDirectory, "download-dir", "d", defaultDownloadDir, "location of downloaded files")
	root.PersistentFlags().StringVarP(&config.Siad.WalletFile, "wallet-file", "w", defaultWalletFile, "location of the wallet file")

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
		if err := gcfg.ReadFileInto(&config, config.Siad.ConfigFilename); err != nil {
			fmt.Println("Failed to load config file:", err)
			return
		}
	}

	// Parse cmdline flags, overwriting both the default values and the config
	// file values.
	root.Execute()
}
