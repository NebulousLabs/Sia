package main

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/sia"

	"github.com/spf13/cobra"
)

type walletEnvironment struct {
	state   *sia.State
	wallets []*sia.Wallet
}

func walletStart(cmd *cobra.Command, args []string) {
	// state := sia.BootstrapToNetwork()

	fmt.Println("Wallet bootstrapping not implemented.")
}

// Creates a new network using sia's genesis tools, then polls using the
// standard function.
func genesisStart(cmd *cobra.Command, args []string) {
	fmt.Println("Creating a new wallet and blockchain...")

	var env walletEnvironment

	wallet, err := sia.CreateWallet()
	if err != nil {
		fmt.Println(err)
		return
	}
	env.wallets = append(env.wallets, wallet)

	env.state = sia.CreateGenesisState(wallet.SpendConditions.CoinAddress())
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("New blockchain created.")

	pollHome(env)
}

func main() {
	// Create the basic command.
	root := &cobra.Command{
		Use:   "sia-cli",
		Short: "Sia Cli v0.1.0",
		Long:  "Sia command line wallet version 0.1.0",
		Run:   walletStart,
	}

	// Create a version command.
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Prints version information about the Sia command line wallet.",
		Run:   func(_ *cobra.Command, _ []string) { fmt.Println("Sia Command Line Wallet v0.1.0") },
	})

	// Create a genesis command.
	root.AddCommand(&cobra.Command{
		Use:   "genesis",
		Short: "Create a genesis block.",
		Long:  "Create a genesis block and begin mining on a new network instead of joining an existing network.",
		Run:   genesisStart,
	})

	root.Execute()
}
