package siacore

import (
	"fmt"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/network"
	"github.com/NebulousLabs/Sia/siacore/wallet"
)

// Environment is the struct that serves as the state for siad. It contains a
// pointer to the state, as things like a wallet, a friend list, etc. Each
// environment should have its own state.
type Environment struct {
	state *consensus.State

	server       *network.TCPServer
	host         *Host
	hostDatabase *HostDatabase
	renter       *Renter
	wallet       Wallet

	friends map[string]consensus.CoinAddress

	// Channels for incoming blocks and transactions to be processed
	blockChan       chan consensus.Block
	transactionChan chan consensus.Transaction

	// Mining variables. The mining variables are protected by the miningLock.
	// Any time that you read from or write to any of the mining variables, you
	// need to be under a lock.
	mining        bool         // true when mining
	miningThreads int          // number of processes mining at once
	miningLock    sync.RWMutex // prevents benign race conditions

	// Envrionment directories.
	hostDir    string
	styleDir   string
	walletFile string
}

// createEnvironment creates a server, host, miner, renter and wallet and
// puts it all in a single environment struct that's used as the state for the
// main package.
func CreateEnvironment(hostDir string, walletFile string, rpcPort uint16, nobootstrap bool) (e *Environment, err error) {
	e = &Environment{
		state:           consensus.CreateGenesisState(),
		friends:         make(map[string]consensus.CoinAddress),
		blockChan:       make(chan consensus.Block, 100),
		transactionChan: make(chan consensus.Transaction, 100),
		hostDir:         hostDir,
		walletFile:      walletFile,
	}
	e.hostDatabase = CreateHostDatabase()
	e.host = CreateHost()
	e.renter = CreateRenter()
	e.wallet, err = wallet.New(e.walletFile)
	if err != nil {
		return
	}

	// Bootstrap to the network.
	err = e.initializeNetwork(rpcPort, nobootstrap)
	if err == network.ErrNoPeers {
		// log.Println("Warning: no peers responded to bootstrap request. Add peers manually to enable bootstrapping.")
	} else if err != nil {
		return
	}
	e.host.Settings.IPAddress = e.server.NetAddress()

	return
}

// Close does any finishing maintenence before the environment can be garbage
// collected. Right now that just means closing the server.
func (e *Environment) Close() {
	e.server.Close()
}

// initializeNetwork registers the rpcs and bootstraps to the network,
// downlading all of the blocks and establishing a peer list.
func (e *Environment) initializeNetwork(rpcPort uint16, nobootstrap bool) (err error) {
	e.server, err = network.NewTCPServer(rpcPort)
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
		go e.CatchUp(e.server.RandomPeer())

		// Every 2 minutes call CatchUp() on a random peer. This will help to
		// resolve synchronization issues and keep everybody on the same page
		// with regards to the longest chain. It's a bit of a hack but will
		// make the network substantially more robust.
		for {
			time.Sleep(time.Minute * 2)
			go e.CatchUp(e.RandomPeer())
		}
	}()

	go e.listen()

	return nil
}

// AddPeer adds a peer.
func (e *Environment) AddPeer(addr network.NetAddress) {
	e.server.AddPeer(addr)
}

// RemovePeer removes a peer.
func (e *Environment) RemovePeer(addr network.NetAddress) {
	e.server.RemovePeer(addr)
}

// AcceptBlock sends the input block down a channel, where it will be dealt
// with by the Environment's listener.
func (e *Environment) AcceptBlock(b consensus.Block) error {
	e.blockChan <- b
	return nil
}

// AcceptTransaction sends the input transaction down a channel, where it will
// be dealt with by the Environment's listener.
func (e *Environment) AcceptTransaction(t consensus.Transaction) error {
	e.transactionChan <- t
	return nil
}

// processBlock is called by the environment's listener.
func (e *Environment) processBlock(b consensus.Block) (err error) {
	e.state.Lock()
	e.hostDatabase.Lock()
	e.host.Lock()
	defer e.state.Unlock()
	defer e.hostDatabase.Unlock()
	defer e.host.Unlock()

	initialStateHeight := e.state.Height()
	rewoundBlockIDs, appliedBlockIDs, outputDiffs, err := e.state.AcceptBlock(b)
	if err == consensus.BlockKnownErr || err == consensus.KnownOrphanErr {
		return
	} else if err != nil {
		// Call CatchUp() if an unknown orphan is sent.
		if err == consensus.UnknownOrphanErr {
			go e.CatchUp(e.server.RandomPeer())
		}
		return
	}

	err = e.wallet.Update(outputDiffs)
	if err != nil {
		return
	}
	e.updateHostDB(rewoundBlockIDs, appliedBlockIDs)
	e.storageProofMaintenance(initialStateHeight, rewoundBlockIDs, appliedBlockIDs)

	// Broadcast all valid blocks.
	go e.server.Broadcast("AcceptBlock", b, nil)
	return
}

// processTransaction sends a transaction to the state.
func (e *Environment) processTransaction(t consensus.Transaction) {
	e.state.Lock()
	defer e.state.Unlock()

	err := e.state.AcceptTransaction(t)
	if err != nil {
		if err != consensus.ConflictingTransactionErr {
			// TODO: Change this println to a logging statement.
			fmt.Println("AcceptTransaction Error:", err)
		}
		return
	}

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
