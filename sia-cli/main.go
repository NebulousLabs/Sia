package main

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/sia"

	"github.com/spf13/cobra"
)

func walletStart(cmd *cobra.Command, args []string) {
	// Create a state object, and then bootstrap to the network using the
	// hardcoded starting addresses.

	fmt.Println("You are trying to start the wallet!.")
}

func genesisStart(cmd *cobra.Command, args []string) {
	wallet, err := sia.CreateWallet()
	if err != nil {
		fmt.Println(err)
		return
	}

	state := sia.CreateGenesisState(wallet.SpendConditions.CoinAddress())
	genesisBlock := sia.CreateGenesisBlock(wallet.SpendConditions.CoinAddress())
	state.AcceptBlock(*genesisBlock)

	fmt.Println("Success!")
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
