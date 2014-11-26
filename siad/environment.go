package siad

import (
	"fmt"

	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

type Environment struct {
	state *siacore.State

	server   *network.TCPServer
	caughtUp bool // False while downloading blocks.

	// host   *Host
	miner *Miner
	// renter *Renter
	wallet *Wallet

	friends map[string]siacore.CoinAddress
}

func (e *Environment) initializeNetwork() (err error) {
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

		e.caughtUp = true
	}()

	return nil
}

// createEnvironment() creates a server, host, miner, renter and wallet and
// puts it all in a single environment struct that's used as the state for the
// main package.
func CreateEnvironment() (e *Environment, err error) {
	e = new(Environment)
	err = e.initializeNetwork()
	if err != nil {
		return
	}
	e.wallet, err = CreateWallet()
	if err != nil {
		fmt.Println(err)
		return
	}
	e.miner = CreateMiner(e.wallet.SpendConditions.CoinAddress())
	// e.host = CreateHost()
	// e.renter = CreateRenter()

	e.miner.state = e.state
	// e.host.state = e.state
	// e.renter.state = e.state
	e.wallet.state = e.state

	// Create the friends map.
	e.friends = make(map[string]siacore.CoinAddress)

	// Accept blocks in a channel. TODO: MAKE IT A GENERAL CHANNEL.
	go func() {
		for {
			e.AcceptBlock(*<-e.miner.blockChan)
		}
	}()

	return
}

func (e *Environment) Close() {
	e.server.Close()
}

// TODO: Handle all accepting of things through a single channel.
func (e *Environment) AcceptBlock(b siacore.Block) (err error) {
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

// TODO: Handle all accepting of things through a single channel.
func (e *Environment) AcceptTransaction(t siacore.Transaction) (err error) {
	err = e.state.AcceptTransaction(t)
	if err != nil {
		fmt.Println("AcceptTransaction Error:", err)
		return
	}
	e.server.Announce('T', t)

	return
}
