package main

import (
	"os"

	"github.com/spf13/cobra"
)

const (
	// Version of the siakg program.
	VERSION = "1.0"
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
		KeyName      string
		RequiredKeys int
		TotalKeys    int
	}
}

// The main function initializes the cobra command scheme and program flags.
func main() {
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia Keygen v" + VERSION,
		Long:  "Sia Keygen v" + VERSION,
		Run:   generateKeys,
	}

	root.PersistentFlags().StringVarP(&config.Siakg.KeyName, "key-name", "n", "SiafundKeys", "The name for this set of keys")
	root.PersistentFlags().IntVarP(&config.Siakg.RequiredKeys, "required-keys", "r", 2, "The number of keys required to use the address")
	root.PersistentFlags().IntVarP(&config.Siakg.TotalKeys, "total-keys", "t", 3, "The total number of keys that can be used with the address")

	root.Execute()
}
