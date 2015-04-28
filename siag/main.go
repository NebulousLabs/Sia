package main

// main.go defines the command structure using the cobra package. This includes
// configuration variables and flags.

import (
	"os"

	"github.com/spf13/cobra"
)

const (
	// Version of the siag program.
	Version = "1.0"

	DefaultFolder       = ""
	DefaultAddressName  = "SiafundAddress"
	DefaultRequiredKeys = 1
	DefaultTotalKeys    = 1
)

var (
	// A global variable containing all of the configuration information,
	// necessary for interacting with cobra.
	config Config
)

// The Config struct holds all of the configuration variables. The format is
// made to be compatible with gcfg. gcfg is not currently used in the siag
// project, however it helps maintain consistency with the design of siad.
type Config struct {
	Siag struct {
		Folder       string
		AddressName  string
		RequiredKeys int
		TotalKeys    int
	}
}

// The main function initializes the cobra command scheme and program flags.
func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Address Generator v" + Version,
		Long:  "Sia Address Generator v" + Version,
		Run:   siag,
	}
	root.Flags().StringVarP(&config.Siag.Folder, "folder", "f", DefaultFolder, "The folder where the keys will be created")
	root.Flags().StringVarP(&config.Siag.AddressName, "address-name", "n", DefaultAddressName, "The name for this address")
	root.Flags().IntVarP(&config.Siag.RequiredKeys, "required-keys", "r", DefaultRequiredKeys, "The number of keys required to use the address")
	root.Flags().IntVarP(&config.Siag.TotalKeys, "total-keys", "t", DefaultTotalKeys, "The total number of keys that can be used with the address")

	address := &cobra.Command{
		Use:   "keyinfo [filename]",
		Short: "Print information about the key.",
		Long:  "Print the address associated with the key as well as the multisig parameters of the key.",
		Run:   keyInfo,
	}
	root.AddCommand(address)

	root.Execute()
}
