package main

import (
	"fmt"
	"net"

	"github.com/NebulousLabs/Andromeda/siacore"
	"github.com/NebulousLabs/Andromeda/siad"
)

type environment struct {
	state *siacore.State

	host   *siad.Host
	miner  *siad.Miner
	renter *siad.Renter
	wallet *siad.Wallet

	// networking stuff here?
}

func createEnvironment() (env *environment) {
	env = new(environment)

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

	// create genesis state and register it with the server
	env.state = siacore.CreateGenesisState()
	if err = tcps.RegisterRPC('B', env.AcceptBlock); err != nil {
		fmt.Println(err)
		return
	}
	if err = tcps.RegisterRPC('T', env.AcceptTransaction); err != nil {
		fmt.Println(err)
		return
	}
	tcps.RegisterHandler('R', env.SendBlocks)
	env.state.Server = tcps

	// download blocks
	env.state.Bootstrap()

	// Create a miner, provider, renter, and wallet.
	env.miner = siad.CreateMiner()
	env.host = siad.CreateHost()
	env.renter = siad.CreateRenter()
	env.wallet, err = siad.CreateWallet()
	if err != nil {
		fmt.Println(err)
		return
	}

	return
}

func (e *environment) AcceptBlock(b siacore.Block) (err error) {
	err = e.state.AcceptBlock(b)
	return
}

func (e *environment) AcceptTransaction(t siacore.Transaction) (err error) {
	err = e.state.AcceptTransaction(t)
	return
}

func (e *environment) SendBlocks(conn net.Conn, data []byte) (err error) {
	err = e.state.SendBlocks(conn, data)
	return
}
