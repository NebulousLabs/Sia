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

// Creates the genesis state and then requests a bunch of blocks from the
// network.
func walletStart(cmd *cobra.Command, args []string) {
	// create TCP server
	tcps, err := sia.NewTCPServer(9988)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer tcps.Close()

	// establish an initial peer list
	if err = tcps.Bootstrap(); err != nil {
		fmt.Println(err)
		return
	}

	// create genesis state and register it with the server
	state := sia.CreateGenesisState(sia.CoinAddress{})
	if err = tcps.RegisterRPC('B', state.AcceptBlock); err != nil {
		fmt.Println(err)
		return
	}
	if err = tcps.RegisterRPC('T', state.AcceptTransaction); err != nil {
		fmt.Println(err)
		return
	}
	state.Server = tcps

	// download blocks
	state.Bootstrap()

	// start generating and sending blocks
}

// Creates a new network using sia's genesis tools, then polls using the
// standard function.
func genesisStart(cmd *cobra.Command, args []string) {
	fmt.Println("Creating a new wallet and blockchain...")

	env := new(walletEnvironment)

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
