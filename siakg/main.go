package main

// main.go defines the command structure using the cobra package. This includes
// configuration variables and flags.

import (
	"os"

	"github.com/spf13/cobra"
)

const (
	// Version of the siakg program.
	Version       = "1.0"
	FileExtension = ".siakey"

	DefaultFolder       = ""
	DefaultKeyname      = "SiafundKeys"
	DefaultRequiredKeys = 2
	DefaultTotalKeys    = 3
)

var (
	// A global variable containing all of the configuration information,
	// necessary for interacting with cobra.
	config Config
)

// The Config struct holds all of the configuration variables. The format is
// made to be compatible with gcfg. gcfg is not currently used in the siakg
// project, however it helps maintain consistency with the design of siad.
type Config struct {
	Siakg struct {
		Folder       string
		KeyName      string
		RequiredKeys int
		TotalKeys    int
	}

	KeyInfo struct {
		Filename string
	}
}

// The main function initializes the cobra command scheme and program flags.
func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Keygen v" + Version,
		Long:  "Sia Keygen v" + Version,
		Run:   siakg,
	}
	root.Flags().StringVarP(&config.Siakg.Folder, "folder", "f", DefaultFolder, "The folder where the keys will be created")
	root.Flags().StringVarP(&config.Siakg.KeyName, "key-name", "n", DefaultKeyname, "The name for this set of keys")
	root.Flags().IntVarP(&config.Siakg.RequiredKeys, "required-keys", "r", DefaultRequiredKeys, "The number of keys required to use the address")
	root.Flags().IntVarP(&config.Siakg.TotalKeys, "total-keys", "t", DefaultTotalKeys, "The total number of keys that can be used with the address")

	address := &cobra.Command{
		Use:   "keyinfo",
		Short: "Print the address associated with a keyfile.",
		Long:  "Load a keyfile and print the address that the keyfile is meant to spend on.",
		Run:   keyInfo,
	}
	address.Flags().StringVarP(&config.KeyInfo.Filename, "filename", "f", "SiafundKeys_Key0"+FileExtension, "Which file is being printed")
	root.AddCommand(address)

	root.Execute()
}
