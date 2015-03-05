package gateway

import (
	"github.com/NebulousLabs/Sia/modules"
)

const (
	sharedPeers   = 10
	maxAddrLength = 100
)

// SharePeers returns up to 10 randomly selected peers.
func (g *Gateway) SharePeers(conn modules.NetConn) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var peers []modules.NetAddress
	for peer := range g.peers {
		if len(peers) == sharedPeers {
			break
		}
		peers = append(peers, peer)
	}
	return conn.WriteObject(peers)
}

// requestPeers calls the SharePeers RPC to learn about new peers. Each
// returned peer is pinged to ensure connectivity, and then added to the peer
// list. Each ping is performed in its own goroutine, which manages its own
// mutexes.
//
// TODO: maybe iterate until we have enough new peers?
func (g *Gateway) requestPeers(addr modules.NetAddress) error {
	var newPeers []modules.NetAddress
	err := g.RPC(addr, "SharePeers", func(conn modules.NetConn) error {
		return conn.ReadObject(&newPeers, sharedPeers*maxAddrLength)
	})
	if err != nil {
		return err
	}
	for _, peer := range newPeers {
		// don't add ourselves
		if peer == g.myAddr {
			continue
		}
		// ping each peer in a separate goroutine
		go func(peer modules.NetAddress) {
			if g.Ping(peer) {
				g.AddPeer(peer)
			}
		}(peer)
	}
	return nil
}

// AddPeer is an RPC that requests that the Gateway add a peer to its peer
// list. The supplied peer is assumed to be the peer making the RPC.
func (g *Gateway) AddMe(conn modules.NetConn) error {
	var peer modules.NetAddress
	err := conn.ReadObject(&peer, maxAddrLength)
	if err != nil {
		return err
	}
	if !g.Ping(peer) {
		return ErrUnreachable
	}
	g.AddPeer(peer)
	return nil
}

// AddPeer adds a peer to the Gateway's peer list.
func (g *Gateway) AddPeer(peer modules.NetAddress) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.addPeer(peer)
}

// RemovePeer removes a peer from the Gateway's peer list.
func (g *Gateway) RemovePeer(peer modules.NetAddress) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.removePeer(peer)
}
