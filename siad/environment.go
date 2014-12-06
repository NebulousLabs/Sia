package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/NebulousLabs/Andromeda/network"
	"github.com/NebulousLabs/Andromeda/siacore"
)

// Environment is the struct that serves as the state for siad. It contains a
// pointer to the state, as things like a wallet, a friend list, etc. Each
// environment should have its own state.
type Environment struct {
	state *siacore.State

	server       *network.TCPServer
	host         *Host
	hostDatabase *HostDatabase
	renter       *Renter
	wallet       *Wallet

	friends map[string]siacore.CoinAddress

	// Channels for incoming blocks and transactions to be processed
	blockChan       chan siacore.Block
	transactionChan chan siacore.Transaction

	// Mining variables
	mining        bool         // true when mining
	miningThreads int          // number of processes mining at once
	miningLock    sync.RWMutex // prevents benign race conditions
}

// createEnvironment creates a server, host, miner, renter and wallet and
// puts it all in a single environment struct that's used as the state for the
// main package.
func CreateEnvironment(port uint16, nobootstrap bool) (e *Environment, err error) {
	e = &Environment{
		state:           siacore.CreateGenesisState(),
		friends:         make(map[string]siacore.CoinAddress),
		blockChan:       make(chan siacore.Block, 100),
		transactionChan: make(chan siacore.Transaction, 100),
	}

	e.hostDatabase = CreateHostDatabase()
	e.host = CreateHost()
	e.renter = CreateRenter()
	e.wallet = CreateWallet(e.state)

	err = e.initializeNetwork(port, nobootstrap)
	if err != nil {
		return
	}

	return
}

// Close does any finishing maintenence before the environment can be garbage
// collected. Right now that just means closing the server.
func (e *Environment) Close() {
	e.server.Close()
}

// initializeNetwork registers the rpcs and bootstraps to the network,
// downlading all of the blocks and establishing a peer list.
func (e *Environment) initializeNetwork(port uint16, nobootstrap bool) (err error) {
	e.server, err = network.NewTCPServer(port)
	if err != nil {
		return
	}

	e.server.Register("AcceptBlock", e.AcceptBlock)
	e.server.Register("AcceptTransaction", e.AcceptTransaction)
	e.server.Register("SendBlocks", e.SendBlocks)
	e.server.Register("NegotiateContract", e.NegotiateContract)
	e.server.Register("RetrieveFile", e.RetrieveFile)

	if nobootstrap {
		go e.listen()
		return
	}

	// establish an initial peer list
	if err = e.server.Bootstrap(); err != nil {
		return
	}

	// Download the blockchain, getting blocks one batch at a time until an
	// empty batch is sent.
	go func() {
		// Catch up the first time.
		if err := e.CatchUp(e.RandomPeer()); err != nil {
			fmt.Println("Error during CatchUp:", err)
		}

		// Every 2 minutes call CatchUp() on a random peer. This will help to
		// resolve synchronization issues and keep everybody on the same page
		// with regards to the longest chain. It's a bit of a hack but will
		// make the network substantially more robust.
		for {
			time.Sleep(time.Minute * 2)
			e.CatchUp(e.RandomPeer())
		}
	}()

	go e.listen()

	return nil
}

// AcceptBlock sends the input block down a channel, where it will be dealt
// with by the Environment's listener.
func (e *Environment) AcceptBlock(b siacore.Block) error {
	e.blockChan <- b
	return nil
}

// AcceptTransaction sends the input transaction down a channel, where it will
// be dealt with by the Environment's listener.
func (e *Environment) AcceptTransaction(t siacore.Transaction) error {
	e.transactionChan <- t
	return nil
}

// processBlock is called by the environment's listener.
func (e *Environment) processBlock(b siacore.Block) {
	// Pass the block to the state, grabbing a lock on hostDatabase and host
	// before releasing the state lock, to ensure that rewoundBlocks and
	// appliedBlocks are managed in the correct order.
	e.state.Lock()
	rewoundBlocks, appliedBlocks, err := e.state.AcceptBlock(b)
	stateHeight := e.state.Height()
	e.hostDatabase.Lock()
	defer e.hostDatabase.Unlock()
	e.host.Lock()
	defer e.host.Unlock()
	e.state.Unlock()

	// Perform error handling.
	if err == siacore.BlockKnownErr {
		// Nothing happens if the block is known.
		return
	} else if err != nil {
		// Call CatchUp() if an unknown orphan is sent.
		if err == siacore.UnknownOrphanErr {
			err = e.CatchUp(e.server.RandomPeer())
			if err != nil {
				// Logging
				// fmt.Println(err2)
			}
		} else if err != siacore.KnownOrphanErr {
			// TODO: Change this from a print statement to a logging statement.
			fmt.Println("AcceptBlock Error: ", err)
		}
		return
	}

	// TODO: once a block has been moved into the host db, it doesn't come out.
	// But the host db should reverse when there are reorgs.
	e.updateHostDB(rewoundBlocks, appliedBlocks)

	e.storageProofMaintenance(stateHeight, rewoundBlocks, appliedBlocks)

	// Broadcast all valid blocks.
	go e.server.Broadcast("AcceptBlock", b, nil)
}

// processTransaction is called by the environment's listener.
func (e *Environment) processTransaction(t siacore.Transaction) {
	// Pass the transaction to the state.
	e.state.Lock()
	err := e.state.AcceptTransaction(t)
	e.state.Unlock()

	// Perform error handling.
	if err != nil {
		if err != siacore.ConflictingTransactionErr {
			// TODO: Change this println to a logging statement.
			fmt.Println("AcceptTransaction Error:", err)
		}
		return
	}

	// Broadcast all valid transactions.
	go e.server.Broadcast("AcceptTransaction", t, nil)
}

// listen waits until a new block or transaction arrives, then attempts to
// process and rebroadcast it.
func (e *Environment) listen() {
	for {
		select {
		case b := <-e.blockChan:
			e.processBlock(b)

		case t := <-e.transactionChan:
			e.processTransaction(t)
		}
	}
}
