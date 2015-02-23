package gateway

import (
	"github.com/NebulousLabs/Sia/network"
)

// SharePeers returns up to 10 randomly selected peers.
func (g *Gateway) SharePeers() (peers []network.Address, err error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for peer := range g.peers {
		if len(peers) > 10 {
			return
		}
		peers = append(peers, peer)
	}
	return
}

// AddPeer is an RPC that requests that the Gateway add a peer to its peer
// list. The supplied peer is assumed to be the peer making the RPC.
func (g *Gateway) AddMe(peer network.Address) error {
	if !network.Ping(peer) {
		return ErrUnreachable
	}
	g.AddPeer(peer)
	return nil
}

// AddPeer adds a peer to the Gateway's peer list.
func (g *Gateway) AddPeer(peer network.Address) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.addPeer(peer)
}

// RemovePeer removes a peer from the Gateway's peer list.
func (g *Gateway) RemovePeer(peer network.Address) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.removePeer(peer)
}
