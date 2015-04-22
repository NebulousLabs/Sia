package gateway

import (
	"errors"
	"math/rand"
	"net"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	maxSharedPeers = 10
	maxAddrLength  = 100
	minPeers       = 3
)

// addNode adds an address to the set of nodes on the network.
func (g *Gateway) addNode(addr modules.NetAddress) error {
	if _, exists := g.nodes[addr]; exists {
		return errors.New("peer already added")
	}
	g.nodes[addr] = struct{}{}
	g.save()
	return nil
}

func (g *Gateway) removePeer(peer modules.NetAddress) error {
	if _, exists := g.peers[peer]; !exists {
		return errors.New("no record of that peer")
	}
	delete(g.peers, peer)
	g.save()
	g.log.Println("INFO: removed peer", peer)
	return nil
}

func (g *Gateway) randomPeer() (*Peer, error) {
	if len(g.peers) > 0 {
		r := rand.Intn(len(g.peers))
		for _, peer := range g.peers {
			if r == 0 {
				return peer, nil
			}
			r--
		}
	}

	return nil, errNoPeers
}

// RemovePeer removes a peer from the Gateway's peer list.
// TODO: warn if less than minPeers?
func (g *Gateway) RemovePeer(peer modules.NetAddress) error {
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	return g.removePeer(peer)
}

// requestPeers calls the SharePeers RPC on p.
func (g *Gateway) requestPeers(p *Peer) error {
	g.log.Printf("INFO: requesting peers from %v\n", p.sess.RemoteAddr())
	var newPeers []modules.NetAddress
	err := p.rpc("SharePeers", readerRPC(&newPeers, maxSharedPeers*maxAddrLength))
	if err != nil {
		return err
	}
	g.log.Printf("INFO: %v sent us %v peers\n", p.sess.RemoteAddr(), len(newPeers))
	for i := range newPeers {
		g.addNode(newPeers[i])
	}
	return nil
}

// sharePeers is an RPC that returns up to 10 randomly selected peers.
func (g *Gateway) sharePeers(conn net.Conn) error {
	id := g.mu.RLock()
	var peers []modules.NetAddress
	for peer := range g.peers {
		if len(peers) == maxSharedPeers {
			break
		}
		peers = append(peers, peer)
	}
	g.mu.RUnlock(id)
	return encoding.WriteObject(conn, peers)
}
