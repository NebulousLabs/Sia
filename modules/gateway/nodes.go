package gateway

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
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
	} else if net.ParseIP(addr.Host()) == nil {
		return errors.New("address is not routable: " + string(addr))
	} else if net.ParseIP(addr.Host()).IsLoopback() {
		return errors.New("cannot add loopback address")
	}
	g.nodes[addr] = struct{}{}
	return nil
}

func (g *Gateway) removeNode(addr modules.NetAddress) error {
	if _, exists := g.nodes[addr]; !exists {
		return errors.New("no record of that node")
	}
	delete(g.nodes, addr)
	g.log.Println("INFO: removed node", addr)
	return nil
}

func (g *Gateway) randomNode() (modules.NetAddress, error) {
	if len(g.nodes) > 0 {
		r, _ := crypto.RandIntn(len(g.nodes))
		for node := range g.nodes {
			if r <= 0 {
				return node, nil
			}
			r--
		}
	}

	return "", errNoPeers
}

// shareNodes is the receiving end of the ShareNodes RPC. It writes up to 10
// randomly selected nodes to the caller.
func (g *Gateway) shareNodes(conn modules.PeerConn) error {
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

// requestNodes is the calling end of the ShareNodes RPC.
func (g *Gateway) requestNodes(conn modules.PeerConn) error {
	var nodes []modules.NetAddress
	if err := encoding.ReadObject(conn, &nodes, maxSharedNodes*maxAddrLength); err != nil {
		return err
	}
	g.log.Printf("INFO: %v sent us %v nodes", conn.RemoteAddr(), len(nodes))
	id := g.mu.Lock()
	for _, node := range nodes {
		g.addNode(node)
	}
	g.save()
	g.mu.Unlock(id)
	return nil
}

// relayNode is the recipient end of the RelayNode RPC. It reads a node, adds
// it to the Gateway's node list, and relays it to each of the Gateway's
// peers. If the node is already in the node list, it is not relayed.
func (g *Gateway) relayNode(conn modules.PeerConn) error {
	// read address
	var addr modules.NetAddress
	if err := encoding.ReadObject(conn, &addr, maxAddrLength); err != nil {
		return err
	}
	// add node
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	if err := g.addNode(addr); err != nil {
		return err
	}
	g.save()
	// relay
	go g.Broadcast("RelayNode", addr)
	return nil
}

// sendAddress is the calling end of the RelayNode RPC.
func (g *Gateway) sendAddress(conn modules.PeerConn) error {
	// don't send if we aren't connectible
	if g.Address().Host() == "::1" {
		return errors.New("can't send address without knowing external IP")
	}
	return encoding.WriteObject(conn, g.Address())
}

// nodeManager tries to keep the Gateway's node list healthy. As long as the
// Gateway has fewer than minNodeListSize nodes, it asks a random peer for
// more nodes. It also continually pings nodes in order to establish their
// connectivity. Unresponsive nodes are aggressively removed.
func (g *Gateway) nodeManager() {
	for {
		time.Sleep(5 * time.Second)

		id := g.mu.RLock()
		numNodes := len(g.nodes)
		peer, err := g.randomPeer()
		g.mu.RUnlock(id)
		if err != nil {
			// can't do much until we have peers
			continue
		}

		if numNodes < minNodeListLen {
			g.RPC(peer, "ShareNodes", g.requestNodes)
		}

		// find an untested node to check
		id = g.mu.RLock()
		node, err := g.randomNode()
		g.mu.RUnlock(id)
		if err != nil {
			continue
		}

		// try to connect
		conn, err := net.DialTimeout("tcp", string(node), dialTimeout)
		if err != nil {
			id = g.mu.Lock()
			g.removeNode(node)
			g.save()
			g.mu.Unlock(id)
			continue
		}
		// if connection succeeds, supply an unacceptable version to ensure
		// they won't try to add us as a peer
		encoding.WriteObject(conn, "0.0.0")
		conn.Close()
	}
}
