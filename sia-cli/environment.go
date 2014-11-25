package main

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
	"github.com/NebulousLabs/Andromeda/siad"
)

type environment struct {
	state *siacore.State

	server *network.TCPServer

	host   *siad.Host
	miner  *siad.Miner
	renter *siad.Renter
	wallet *siad.Wallet
}

func (e *environment) initializeNetwork() (err error) {
	e.server, err = network.NewTCPServer(9988)
	if err != nil {
		return
	}

	// establish an initial peer list
	if err = e.server.Bootstrap(); err != nil {
		fmt.Println(err)
		return
	}

	// create genesis state and register it with the server
	e.state = siacore.CreateGenesisState()
	e.server.Register('B', e.AcceptBlock)
	e.server.Register('T', e.AcceptTransaction)
	e.server.Register('R', e.state.SendBlocks)

	// Get a peer to download the blockchain from.
	randomPeer := e.server.RandomPeer()
	fmt.Println(randomPeer)

	// Download the blockchain, getting blocks one batch at a time until an
	// empty batch is sent.
	go func() {
		for {
			prevHeight := e.state.Height()
			err = e.state.CatchUp(randomPeer)

			if err != nil {
				fmt.Println("Error during CatchUp:", err)
				break
			}

			if prevHeight == e.state.Height() {
				break
			}
		}
	}()

	return nil
}

// createEnvironment() creates a server, host, miner, renter and wallet and
// puts it all in a single environment struct that's used as the state for the
// main package.
func createEnvironment() (env *environment, err error) {
	env = new(environment)
	err = env.initializeNetwork()
	if err != nil {
		return
	}
	env.miner = siad.CreateMiner()
	env.host = siad.CreateHost()
	env.renter = siad.CreateRenter()
	env.wallet, err = siad.CreateWallet()
	if err != nil {
		fmt.Println(err)
		return
	}

	// accept mined blocks
	// TODO: WHEN SHOULD THIS TERMINATE?
	go func() {
		for {
			env.AcceptBlock(*<-env.miner.BlockChan)
		}
	}()

	return
}

func (e *environment) Close() {
	e.server.Close()
}

func (e *environment) AcceptBlock(b siacore.Block) (err error) {
	err = e.state.AcceptBlock(b)
	if err != nil && err != siacore.BlockKnownErr {
		fmt.Println("AcceptBlock Error: ", err)
		if err == siacore.UnknownOrphanErr {
			err2 := e.state.CatchUp(e.server.RandomPeer())
			if err2 != nil {
				// Logging
				// fmt.Println(err2)
			}
		}
		return
	}
	go e.server.Announce('B', b)

	return
}

func (e *environment) AcceptTransaction(t siacore.Transaction) (err error) {
	err = e.state.AcceptTransaction(t)
	if err != nil {
		fmt.Println("AcceptTransaction Error:", err)
		return
	}
	e.server.Announce('T', t)

	return
}
