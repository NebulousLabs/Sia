package gateway

import (
	"errors"
	"math/rand"

	"github.com/NebulousLabs/Sia/modules"
)

const (
	maxSharedPeers = 10
	maxAddrLength  = 100
	minPeers       = 3
)

func (g *Gateway) addPeer(peer modules.NetAddress) error {
	if _, exists := g.peers[peer]; exists {
		return errors.New("peer already added")
	} else if peer == g.myAddr {
		return errors.New("can't add our own address")
	}
	// If adding this peer brings us above minPeers, try to kick out a bad
	// peer that we've been forced to keep.
	if len(g.peers) == minPeers {
		for peer, strikes := range g.peers {
			if strikes > maxStrikes {
				g.removePeer(peer)
				break
			}
		}
	}
	g.peers[peer] = 0
	g.save()

	g.log.Println("INFO: added new peer", peer)
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

func (g *Gateway) randomPeer() (modules.NetAddress, error) {
	if len(g.peers) > 0 {
		r := rand.Intn(len(g.peers))
		for peer := range g.peers {
			if r == 0 {
				return peer, nil
			}
			r--
		}
	}

	return "", errNoPeers
}

func (g *Gateway) addStrike(peer modules.NetAddress) {
	if _, exists := g.peers[peer]; !exists {
		return
	}
	g.peers[peer]++
	g.log.Println("INFO: added a strike to peer", peer)
	// don't remove bad peers if we aren't well-connected
	if g.peers[peer] > maxStrikes && len(g.peers) > minPeers {
		g.removePeer(peer)
	}
}

// AddPeer adds a peer to the Gateway's peer list.
func (g *Gateway) AddPeer(peer modules.NetAddress) error {
	counter := g.mu.Lock()
	defer g.mu.Unlock(counter)
	return g.addPeer(peer)
}

// RemovePeer removes a peer from the Gateway's peer list.
// TODO: warn if less than minPeers?
func (g *Gateway) RemovePeer(peer modules.NetAddress) error {
	counter := g.mu.Lock()
	defer g.mu.Unlock(counter)
	return g.removePeer(peer)
}

// RandomPeer returns a random peer from the Gateway's peer list.
func (g *Gateway) RandomPeer() (modules.NetAddress, error) {
	counter := g.mu.RLock()
	defer g.mu.RUnlock(counter)
	return g.randomPeer()
}

// addMe is an RPC that requests that the Gateway add a peer to its peer
// list. The supplied peer is assumed to be the peer making the RPC.
func (g *Gateway) addMe(conn modules.NetConn) error {
	var peer modules.NetAddress
	err := conn.ReadObject(&peer, maxAddrLength)
	if err != nil {
		return err
	}
	g.log.Printf("INFO: %v wants to add %v to our peer list\n", conn.Addr(), peer)
	if !g.Ping(peer) {
		return errUnreachable
	}
	g.AddPeer(peer)
	return nil
}

// sharePeers is an RPC that returns up to 10 randomly selected peers.
func (g *Gateway) sharePeers(conn modules.NetConn) error {
	counter := g.mu.RLock()
	defer g.mu.RUnlock(counter)

	var peers []modules.NetAddress
	for peer := range g.peers {
		if len(peers) == maxSharedPeers {
			break
		}
		// don't send requester their own address
		if peer == conn.Addr() {
			continue
		}
		peers = append(peers, peer)
	}
	return conn.WriteObject(peers)
}

// requestPeers calls the SharePeers RPC on addr and returns the response.
func (g *Gateway) requestPeers(addr modules.NetAddress) (newPeers []modules.NetAddress, err error) {
	g.log.Printf("INFO: requesting peers from %v\n", addr)
	err = g.RPC(addr, "SharePeers", readerRPC(&newPeers, maxSharedPeers*maxAddrLength))
	g.log.Printf("INFO: %v sent us %v peers\n", addr, len(newPeers))
	return
}

// threadedPeerDiscovery calls requestPeers on each peer in the current peer
// list and adds all of the returned peers.
func (g *Gateway) threadedPeerDiscovery() {
	var newPeers []modules.NetAddress
	for _, peer := range g.Info().Peers {
		resp, err := g.requestPeers(peer)
		if err != nil {
			continue
		}
		newPeers = append(newPeers, resp...)
	}

	id := g.mu.Lock()
	var added int
	for i := range newPeers {
		if g.addPeer(newPeers[i]) == nil {
			added++
		}
	}
	g.mu.Unlock(id)

	if added == 0 {
		g.log.Println("WARN: peer discovery found no new peers")
		return
	}

	// announce ourselves to the new peers
	g.threadedBroadcast("AddMe", writerRPC(g.Address()))
}
