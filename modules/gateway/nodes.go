package gateway

import (
	"errors"
	"math/rand"
	"net"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	maxSharedNodes = 10
	maxAddrLength  = 100
	minPeers       = 3
)

// addNode adds an address to the set of nodes on the network.
func (g *Gateway) addNode(addr modules.NetAddress) error {
	if _, exists := g.nodes[addr]; exists {
		return errors.New("node already added")
	}
	g.nodes[addr] = struct{}{}
	g.save()
	return nil
}

func (g *Gateway) removeNode(addr modules.NetAddress) error {
	if _, exists := g.nodes[addr]; !exists {
		return errors.New("no record of that node")
	}
	delete(g.nodes, addr)
	g.save()
	g.log.Println("INFO: removed node", addr)
	return nil
}

func (g *Gateway) randomNode() (modules.NetAddress, error) {
	if len(g.nodes) > 0 {
		r := rand.Intn(len(g.nodes))
		for node := range g.nodes {
			if r == 0 {
				return node, nil
			}
			r--
		}
	}

	return "", errNoPeers
}

// requestNodes calls the ShareNodes RPC on p.
func (g *Gateway) requestNodes(p *Peer) error {
	g.log.Printf("INFO: requesting peers from %v\n", p.sess.RemoteAddr())
	var newPeers []modules.NetAddress
	err := p.rpc("ShareNodes", readerRPC(&newPeers, maxSharedNodes*maxAddrLength))
	if err != nil {
		return err
	}
	g.log.Printf("INFO: %v sent us %v peers\n", p.sess.RemoteAddr(), len(newPeers))
	id := g.mu.Lock()
	for i := range newPeers {
		g.addNode(newPeers[i])
	}
	g.mu.Unlock(id)
	return nil
}

// shareNodes is an RPC that returns up to 10 randomly selected nodes.
func (g *Gateway) shareNodes(conn net.Conn) error {
	id := g.mu.RLock()
	var nodes []modules.NetAddress
	for node := range g.nodes {
		if len(nodes) == maxSharedNodes {
			break
		}
		nodes = append(nodes, node)
	}
	g.mu.RUnlock(id)
	return encoding.WriteObject(conn, nodes)
}
