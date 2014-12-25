package sia

import (
	"time"

	"github.com/NebulousLabs/Sia/network"
)

// AddPeer adds a peer.
func (e *Environment) AddPeer(addr network.Address) {
	e.server.AddPeer(addr)
}

// initializeNetwork registers the rpcs and bootstraps to the network,
// downlading all of the blocks and establishing a peer list.
func (e *Environment) initializeNetwork(addr string, nobootstrap bool) (err error) {
	e.server, err = network.NewTCPServer(addr)
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

// RemovePeer removes a peer.
func (e *Environment) RemovePeer(addr network.Address) {
	e.server.RemovePeer(addr)
}
