package sia

import (
	"time"

	"github.com/NebulousLabs/Sia/network"
)

// initializeNetwork registers the rpcs and bootstraps to the network,
// downlading all of the blocks and establishing a peer list.
func (c *Core) initializeNetwork(addr string, nobootstrap bool) (err error) {
	c.server, err = network.NewTCPServer(addr)
	if err != nil {
		return
	}

	err = c.server.RegisterRPC("AcceptBlock", c.state.AcceptBlock)
	if err != nil {
		return
	}
	err = c.server.RegisterRPC("AcceptTransaction", c.state.AcceptTransaction)
	if err != nil {
		return
	}
	err = c.server.RegisterRPC("SendBlocks", c.SendBlocks)
	if err != nil {
		return
	}
	c.server.RegisterRPC("NegotiateContract", c.host.NegotiateContract)
	if err != nil {
		return
	}
	c.server.RegisterRPC("RetrieveFile", c.host.RetrieveFile)
	if err != nil {
		return
	}

	// If we aren't bootstrapping, then we're done.
	// TODO: this means the CatchUp thread isn't spawned.
	// It should probably be spawned after the first peer connects.
	if nobootstrap {
		return
	}

	// Bootstrapping may take a while.
	go func() {
		// Establish an initial peer list.
		if err = c.server.Bootstrap(); err != nil {
			// log error
			return
		}

		// Every 2 minutes, call CatchUp() on a random peer. This will help to
		// resolve synchronization issues and keep everybody on the same page
		// with regards to the longest chain. It's a bit of a hack but will
		// make the network substantially more robust.
		for {
			go c.CatchUp(c.server.RandomPeer())
			time.Sleep(time.Minute * 2)
		}
	}()

	return
}
