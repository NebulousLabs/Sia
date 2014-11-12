package main

import (
	"fmt"
	"os"

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

	env := new(walletEnvironment)

	// create genesis state and register it with the server
	env.state = sia.CreateGenesisState(sia.CoinAddress{})
	if err = tcps.RegisterRPC('B', env.state.AcceptBlock); err != nil {
		fmt.Println(err)
		return
	}
	if err = tcps.RegisterRPC('T', env.state.AcceptTransaction); err != nil {
		fmt.Println(err)
		return
	}
	if err = tcps.RegisterRPC('R', env.state.SendBlocks); err != nil {
		fmt.Println(err)
		return
	}
	env.state.Server = tcps

	// download blocks
	env.state.Bootstrap()

	wallet, err := sia.CreateWallet()
	if err != nil {
		fmt.Println(err)
		return
	}
	env.wallets = append(env.wallets, wallet)

	pollHome(env)
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

	fmt.Println("New blockchain created")

	pollHome(env)
}

func main() {
	// Create the basic command.
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "Sia CLI v0.1.0",
		Long:  "Sia Command Line Wallet, version 0.1.0",
		Run:   walletStart,
	}

	// Create a version command.
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information about the Sia Command Line Wallet.",
		Run:   func(_ *cobra.Command, _ []string) { fmt.Println("Sia Command Line Wallet v0.1.0") },
	})

	// Create a genesis command.
	root.AddCommand(&cobra.Command{
		Use:   "genesis",
		Short: "Create a genesis block",
		Long:  "Create a genesis block and begin mining on a new network instead of joining an existing network",
		Run:   genesisStart,
	})

	root.Execute()
}
