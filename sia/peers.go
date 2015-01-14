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

	c.server.Register("AcceptBlock", c.AcceptBlock)
	c.server.Register("AcceptTransaction", c.AcceptTransaction)
	c.server.Register("SendBlocks", c.SendBlocks)
	// c.server.Register("NegotiateContract", c.NegotiateContract)
	// c.server.Register("RetrieveFile", c.RetrieveFile)

	// Start listener thread
	go c.listen()

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
			go c.CatchUp(c.RandomPeer())
			time.Sleep(time.Minute * 2)
		}
	}()

	return
}

// AddPeer adds a peer.
func (c *Core) AddPeer(addr network.Address) {
	c.server.AddPeer(addr)
}

// RandomPeer provides a random peer from the peer list.
func (c *Core) RandomPeer() network.Address {
	return c.server.RandomPeer()
}

// RemovePeer removes a peer.
func (c *Core) RemovePeer(addr network.Address) {
	c.server.RemovePeer(addr)
}

func (c *Core) AddressBook() []network.Address {
	return c.server.AddressBook()
}

func (c *Core) Address() network.Address {
	return c.server.Address()
}
