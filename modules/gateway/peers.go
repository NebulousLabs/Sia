package gateway

import (
	"errors"
	"math/rand"

	"github.com/NebulousLabs/Sia/modules"

	"github.com/inconshreveable/muxado"
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

// addPeer records the connected peer in the peer list.
func (g *Gateway) addPeer(conn modules.NetConn, addr modules.NetAddress) (*Peer, error) {
	if _, exists := g.peers[addr]; exists {
		return nil, errors.New("peer already added")
	} else if addr == g.myAddr {
		return nil, errors.New("can't connect to our own address")
	}
	// If adding this peer brings us above minPeers, try to kick out a bad
	// peer that we've been forced to keep.
	if len(g.peers) == minPeers {
		for addr, peer := range g.peers {
			if peer.strikes > maxStrikes {
				g.removePeer(addr)
				break
			}
		}
	}
	peer := &Peer{muxado.Server(conn), 0}
	g.peers[addr] = peer
	g.addNode(addr)

	g.log.Println("INFO: added new peer", peer)
	return peer, nil
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

func (g *Gateway) addStrike(addr modules.NetAddress) {
	if _, exists := g.peers[addr]; !exists {
		g.log.Printf("WARN: couldn't add strike to non-peer %v\n", addr)
		return
	}
	g.peers[addr].strikes++
	g.log.Println("INFO: added a strike to peer", addr)
	// don't remove bad peers if we aren't well-connected
	if g.peers[addr].strikes > maxStrikes && len(g.peers) > minPeers {
		g.removePeer(addr)
	}
}

// AddPeer adds a peer to the Gateway's peer list.
func (g *Gateway) AddPeer(addr modules.NetAddress) error {
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	conn, err := dial(addr)
	if err != nil {
		return err
	}
	_, err = g.addPeer(conn, addr)
	return err
}

// RemovePeer removes a peer from the Gateway's peer list.
// TODO: warn if less than minPeers?
func (g *Gateway) RemovePeer(peer modules.NetAddress) error {
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	return g.removePeer(peer)
}

// RandomPeer returns a random peer from the Gateway's peer list.
func (g *Gateway) RandomPeer() (modules.NetAddress, error) {
	id := g.mu.RLock()
	defer g.mu.RUnlock(id)
	return g.randomPeer()
}

// addMe is an RPC that requests that the Gateway add a peer to its peer set.
// addMe is a special RPC in that, if the peer is added, it will block forever,
// listening and handling new streams from the peer. Why not spawn a listener
// goroutine? Because the underlying TCP connection will be closed (by
// threadedHandleConn) when the RPC returns.
func (g *Gateway) addMe(conn modules.NetConn) error {
	var addr modules.NetAddress
	err := conn.ReadObject(&addr, maxAddrLength)
	if err != nil {
		return err
	}
	g.log.Printf("INFO: %v wants to connect (gave address: %v)\n", conn.Addr(), addr)
	id := g.mu.RLock()
	peer, err := g.addPeer(conn, addr)
	g.mu.RUnlock(id)
	if err != nil {
		return err
	}
	// block until connection is closed
	g.listen(peer.sess.NetListener())
	// remove peer
	g.RemovePeer(addr)
	return nil
}

// sharePeers is an RPC that returns up to 10 randomly selected peers.
func (g *Gateway) sharePeers(conn modules.NetConn) error {
	id := g.mu.RLock()
	defer g.mu.RUnlock(id)

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
		if g.addNode(newPeers[i]) == nil {
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
