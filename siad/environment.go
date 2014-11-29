package siad

import (
	"fmt"
	"sync"
	"time"

	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

type Environment struct {
	state *siacore.State

	server       *network.TCPServer
	caughtUp     bool // False while downloading blocks.
	caughtUpLock sync.Mutex

	// host   *Host
	miner *Miner
	// renter *Renter
	wallet *Wallet

	friends map[string]siacore.CoinAddress

	// Channels for incoming blocks/transactions to be processed
	blockChan       chan siacore.Block
	transactionChan chan siacore.Transaction
}

// createEnvironment() creates a server, host, miner, renter and wallet and
// puts it all in a single environment struct that's used as the state for the
// main package.
func CreateEnvironment() (e *Environment, err error) {
	e = &Environment{
		state:           siacore.CreateGenesisState(),
		friends:         make(map[string]siacore.CoinAddress),
		blockChan:       make(chan siacore.Block, 100),
		transactionChan: make(chan siacore.Transaction, 100),
	}

	err = e.initializeNetwork()
	if err != nil {
		return
	}
	e.wallet = CreateWallet(e.state)
	ROblockChan := (chan<- siacore.Block)(e.blockChan)
	e.miner = CreateMiner(e.state, ROblockChan, e.wallet.SpendConditions.CoinAddress())
	// e.host = CreateHost(e.state)
	// e.renter = CreateRenter(e.state)

	return
}

func (e *Environment) Close() {
	e.server.Close()
}

func (e *Environment) initializeNetwork() (err error) {
	e.server, err = network.NewTCPServer(9988)
	if err != nil {
		// TODO: Retry a single time with a different port number. This allows 2
		// instances to be running on the same machine, which is useful for
		// testing. It's hacky.
		e.server, err = network.NewTCPServer(9989)
		if err != nil {
			return
		}
	}

	// establish an initial peer list
	if err = e.server.Bootstrap(); err != nil {
		fmt.Println(err)
		return
	}

	e.server.Register("AcceptBlock", e.AcceptBlock)
	e.server.Register("AcceptTransaction", e.AcceptTransaction)
	e.server.Register("SendBlocks", e.state.SendBlocks)

	// Get a peer to download the blockchain from.
	randomPeer := e.server.RandomPeer()
	fmt.Println(randomPeer)

	// Download the blockchain, getting blocks one batch at a time until an
	// empty batch is sent.
	go func() {
		// Catch up the first time.
		e.state.Lock()
		if err := e.state.CatchUp(randomPeer); err != nil {
			fmt.Println("Error during CatchUp:", err)
		}
		e.state.Unlock()

		e.caughtUpLock.Lock()
		e.caughtUp = true
		e.caughtUpLock.Unlock()

		// Every 2 minutes call CatchUp() on a random peer. This will help to
		// resolve synchronization issues and keep everybody on the same page
		// with regards to the longest chain. It's a bit of a hack but will
		// make the network substantially more robust.
		for {
			time.Sleep(time.Minute * 2)
			e.state.Lock()
			e.state.CatchUp(e.server.RandomPeer())
			e.state.Unlock()
		}
	}()

	go e.listen()

	return nil
}

func (e *Environment) AcceptBlock(b siacore.Block) error {
	e.blockChan <- b
	return nil
}

func (e *Environment) AcceptTransaction(t siacore.Transaction) error {
	e.transactionChan <- t
	return nil
}

// listen waits until a new block or transaction arrives, then attempts to
// process and rebroadcast it.
func (e *Environment) listen() {
	var err error
	for {
		select {
		case b := <-e.blockChan:
			e.state.Lock()
			err = e.state.AcceptBlock(b)
			e.state.Unlock()
			if err == siacore.BlockKnownErr {
				continue
			} else if err != nil {
				if err == siacore.UnknownOrphanErr {
					e.state.Lock()
					err = e.state.CatchUp(e.server.RandomPeer())
					e.state.Unlock()
					if err != nil {
						// Logging
						// fmt.Println(err2)
					}
				} else if err != siacore.KnownOrphanErr {
					fmt.Println("AcceptBlock Error: ", err)
				}
				continue
			}
			go e.server.Broadcast("AcceptBlock", b, nil)

		case t := <-e.transactionChan:
			e.state.Lock()
			err = e.state.AcceptTransaction(t)
			e.state.Unlock()
			if err != nil {
				fmt.Println("AcceptTransaction Error:", err)
				continue
			}
			go e.server.Broadcast("AcceptTransaction", t, nil)
		}
	}
}
