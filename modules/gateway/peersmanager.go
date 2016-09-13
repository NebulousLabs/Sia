package gateway

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

// managedPeerManagerConnect is a blocking function which tries to connect to
// the input addreess as a peer.
func (g *Gateway) managedPeerManagerConnect(addr modules.NetAddress) {
	g.log.Debugln("[PMC] Attempting connection to", addr)
	err := g.managedConnect(addr)
	if err == errPeerExists {
		// This peer is already connected to us. Safety around the
		// oubound peers relates to the fact that we have picked out
		// the outbound peers instead of allow the attacker to pick out
		// the peers for us. Because we have made the selection, it is
		// okay to set the peer as an outbound peer.
		//
		// The nodelist size check ensures that an attacker can't flood
		// a new node with a bunch of inbound requests. Doing so would
		// result in a nodelist that's entirely full of attacker nodes.
		// There's not much we can do about that anyway, but at least
		// we can hold off making attacker nodes 'outbound' peers until
		// our nodelist has had time to fill up naturally.
		g.mu.Lock()
		p, exists := g.peers[addr]
		if exists {
			// Have to check it exists because we released the lock, a
			// race condition could mean that the peer was disconnected
			// before this code block was reached.
			p.Inbound = false
			g.log.Debugln("[PMC] [SUCCESS] existing peer has been converted to outbound peer:", addr)
		} else {
			g.log.Debugln("[PMC] errPeerExists was returned, but by the time the peer was to be set to 'outbound', it no longer existed.")
		}
		g.mu.Unlock()
	} else if err != nil {
		g.log.Debugf("[PMC] WARN: automatic connect to %v failed: %v\n", addr, err)
	} else {
		g.log.Debugln("[PMC] [SUCCESS] addr has been successfully added to the gateway:", addr)
	}
}

// numOutboundPeers returns the number of outbound peers in the gateway.
func (g *Gateway) numOutboundPeers() (numOutboundPeers int) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, p := range g.peers {
		if !p.Inbound {
			numOutboundPeers++
		}
	}
	return numOutboundPeers
}

// permanentPeerManager tries to keep the Gateway well-connected. As long as
// the Gateway is not well-connected, it tries to connect to random nodes.
func (g *Gateway) permanentPeerManager(closedChan chan struct{}) {
	// Send a signal upon shutdown.
	defer close(closedChan)

	// permanentPeerManager will attempt to connect to peers asynchronously,
	// such that multiple connection attempts can be open at once, but a
	// limited number.
	connectionLimiterChan := make(chan struct{}, maxConcurrentOutboundPeerRequests)

	for {
		// If the gateway is well connected, sleep for a while and then try
		// again.
		numOutboundPeers := g.numOutboundPeers()
		if numOutboundPeers >= wellConnectedThreshold {
			if !g.managedSleep(wellConnectedDelay) {
				return
			}
			continue
		}

		// Fetch a random node.
		g.mu.RLock()
		addr, err := g.randomNode()
		g.mu.RUnlock()
		// If there was an error, log the error and then wait a while before
		// trying again.
		if err != nil {
			g.log.Debugln("[PPM] Unable to acquire selected peer:", err)
			if !g.managedSleep(noNodesDelay) {
				return
			}
			continue
		}
		// We need at least some of our outbound peers to be remote peers. If
		// we already have reached a certain threshold of outbound peers and
		// this peer is a local peer, do not consider it for an outbound peer.
		// Sleep breifly to prevent the gateway from hogging the CPU if all
		// peers are local.
		if numOutboundPeers >= maxLocalOutboundPeers && addr.IsLocal() && build.Release != "testing" {
			g.log.Debugln("[PPM] Ignorning selected peer; this peer is local and we already have multiple outbound peers:", addr)
			if !g.managedSleep(unwantedLocalPeerDelay) {
				return
			}
			continue
		}
		g.log.Debugln("[PPM] Gateway does not have enough peers, attempting to acquire a new one:", addr)

		// Try connecting to that peer in a goroutine. Do not block unless
		// there are currently 3 or more peer connection attempts open at once.
		// Before spawning the thread, make sure that there is enough room by
		// throwing a struct into the buffered channel.
		connectionLimiterChan <- struct{}{}
		go func(addr modules.NetAddress) {
			// After completion, take the struct out of the channel so that the
			// next thread may proceed.
			defer func() {
				<-connectionLimiterChan
			}()

			if err := g.threads.Add(); err != nil {
				return
			}
			defer g.threads.Done()
			// peerManagerConnect will handle all of its own logging.
			g.managedPeerManagerConnect(addr)
		}(addr)

		// Wait a bit before trying the next peer. The peer connections are
		// non-blocking, so they should be spaced out to avoid spinning up an
		// uncontrolled number of threads and therefore peer connections.
		if !g.managedSleep(acquiringPeersDelay) {
			return
		}
	}
}
