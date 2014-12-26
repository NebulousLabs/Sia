package sia

import (
	"time"

	"github.com/NebulousLabs/Sia/network"
)

// AddPeer adds a peer.
func (c *Core) AddPeer(addr network.Address) {
	c.server.AddPeer(addr)
}

// initializeNetwork registers the rpcs and bootstraps to the network,
// downlading all of the blocks and establishing a peer list.
func (c *Core) initializeNetwork(addr string, nobootstrap bool) (err error) {
	c.server, err = network.NewTCPServer(addr)
	if err != nil {
		return
	}

	c.server.Register("AcceptBlock", c.AcceptBlock)
	c.server.Register("AcceptTransaction", c.AcceptTransaction)
	c.server.Register("SendBlocks", c.SendBlocks)
	// c.server.Register("NegotiateContract", c.NegotiateContract)
	// c.server.Register("RetrieveFile", c.RetrieveFile)

	if nobootstrap {
		go c.listen()
		return
	}

	// establish an initial peer list
	if err = c.server.Bootstrap(); err != nil {
		return
	}

	// Download the blockchain, getting blocks one batch at a time until an
	// empty batch is sent.
	go func() {
		// Catch up the first time.
		go c.CatchUp(c.server.RandomPeer())

		// Every 2 minutes call CatchUp() on a random peer. This will help to
		// resolve synchronization issues and keep everybody on the same page
		// with regards to the longest chain. It's a bit of a hack but will
		// make the network substantially more robust.
		for {
			time.Sleep(time.Minute * 2)
			go c.CatchUp(c.RandomPeer())
		}
	}()

	go c.listen()

	return nil
}

// RemovePeer removes a peer.
func (c *Core) RemovePeer(addr network.Address) {
	c.server.RemovePeer(addr)
}
