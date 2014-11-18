package main

import (
	"fmt"
	"os"

	"github.com/NebulousLabs/Andromeda/siacore"

	"github.com/spf13/cobra"
)

type walletEnvironment struct {
	state   *siacore.State
	wallets []*siacore.Wallet
}

// Creates the genesis state and then requests a bunch of blocks from the
// network.
func walletStart(cmd *cobra.Command, args []string) {
	// create TCP server
	tcps, err := siacore.NewTCPServer(9988)
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
	env.state = siacore.CreateGenesisState()
	if err = tcps.RegisterRPC('B', env.state.AcceptBlock); err != nil {
		fmt.Println(err)
		return
	}
	if err = tcps.RegisterRPC('T', env.state.AcceptTransaction); err != nil {
		fmt.Println(err)
		return
	}
	tcps.RegisterHandler('R', env.state.SendBlocks)
	env.state.Server = tcps

	// download blocks
	env.state.Bootstrap()

	wallet, err := siacore.CreateWallet()
	if err != nil {
		fmt.Println(err)
		return
	}
	env.wallets = append(env.wallets, wallet)

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

	root.Execute()
}
