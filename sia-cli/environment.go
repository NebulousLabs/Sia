package main

import (
	"fmt"
	"net"

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
	if err = e.server.RegisterRPC('B', e.AcceptBlock); err != nil {
		fmt.Println(err)
		return
	}
	if err = e.server.RegisterRPC('T', e.AcceptTransaction); err != nil {
		fmt.Println(err)
		return
	}
	e.server.RegisterHandler('R', e.SendBlocks)

	// download blockchain
	randomPeer := e.server.RandomPeer()
	for randomPeer.Call(e.state.CatchUp(e.state.Height())) == nil {
	}

	return
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
	// TODO: when should this terminate?
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
	if err != nil {
		fmt.Println("AcceptBlock Error: ", err)
		if err == siacore.UnknownOrphanErr {
			// ASK THE SENDING NODE FOR THE ORPHANS PARENTS.
			peer := e.server.RandomPeer()
			peer.Call(e.state.CatchUp(e.state.Height()))
		}
		return
	}
	go e.server.Broadcast(network.SendVal('B', b))

	return
}

func (e *environment) AcceptTransaction(t siacore.Transaction) (err error) {
	err = e.state.AcceptTransaction(t)
	if err != nil {
		fmt.Println("AcceptTransaction Error:", err)
		return
	}
	e.server.Broadcast(network.SendVal('T', t))

	return
}

func (e *environment) SendBlocks(conn net.Conn, data []byte) (err error) {
	err = e.state.SendBlocks(conn, data)
	return
}
