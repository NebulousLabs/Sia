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
	maxAddrLength  = 100 // TODO: max NetAddress length is 254 for the hostname (including the trailing dot, 253 without) + 1 for the : + 5 for the port.
	minPeers       = 3
)

var (
	errNodeExists = errors.New("node already added")
)

// addNode adds an address to the set of nodes on the network.
func (g *Gateway) addNode(addr modules.NetAddress) error {
	if _, exists := g.nodes[addr]; exists {
		return errNodeExists
	} else if addr.IsValid() != nil {
		return errors.New("address is not valid: " + string(addr))
	} else if net.ParseIP(addr.Host()) == nil {
		return errors.New("address must be an IP address: " + string(addr))
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
	g.mu.RLock()
	var nodes []modules.NetAddress
	for node := range g.nodes {
		if len(nodes) == maxSharedNodes {
			break
		}
		nodes = append(nodes, node)
	}
	g.mu.RUnlock()
	return encoding.WriteObject(conn, nodes)
}

// requestNodes is the calling end of the ShareNodes RPC.
func (g *Gateway) requestNodes(conn modules.PeerConn) error {
	var nodes []modules.NetAddress
	if err := encoding.ReadObject(conn, &nodes, maxSharedNodes*maxAddrLength); err != nil {
		return err
	}
	g.mu.Lock()
	for _, node := range nodes {
		err := g.addNode(node)
		if err != nil {
			g.log.Printf("WARN: peer '%v' send the invalid addr '%v'", conn.RemoteAddr(), node)
		}
	}
	g.save()
	g.mu.Unlock()
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
	err := func() error {
		// We wrap this logic in an anonymous function so we can defer Unlock to
		// avoid managing locks across branching.
		g.mu.Lock()
		defer g.mu.Unlock()
		if err := g.addNode(addr); err != nil {
			return err
		}
		if err := g.save(); err != nil {
			return err
		}
		return nil
	}()
	if err != nil {
		return err
	}
	// relay
	peers := g.Peers()
	go g.Broadcast("RelayNode", addr, peers)
	return nil
}

// sendAddress is the calling end of the RelayNode RPC.
func (g *Gateway) sendAddress(conn modules.PeerConn) error {
	// don't send if we aren't connectible
	if g.Address().IsValid() != nil {
		return errors.New("can't send address without knowing external IP")
	}
	return encoding.WriteObject(conn, g.Address())
}

// threadedNodeManager tries to keep the Gateway's node list healthy. As long
// as the Gateway has fewer than minNodeListSize nodes, it asks a random peer
// for more nodes. It also continually pings nodes in order to establish their
// connectivity. Unresponsive nodes are aggressively removed.
func (g *Gateway) threadedNodeManager() {
	if g.threads.Add() != nil {
		return
	}
	defer g.threads.Done()

	for {
		select {
		case <-time.After(5 * time.Second):
		case <-g.threads.StopChan():
			return
		}

		g.mu.RLock()
		numNodes := len(g.nodes)
		peer, err := g.randomPeer()
		g.mu.RUnlock()
		if err != nil {
			// can't do much until we have peers
			continue
		}

		if numNodes < minNodeListLen {
			err := g.RPC(peer, "ShareNodes", g.requestNodes)
			if err != nil {
				g.log.Debugf("WARN: RPC ShareNodes failed on peer %q: %v", peer, err)
				continue
			}
		}

		// find an untested node to check
		g.mu.RLock()
		node, err := g.randomNode()
		g.mu.RUnlock()
		if err != nil {
			continue
		}

		// try to connect
		conn, err := net.DialTimeout("tcp", string(node), dialTimeout)
		if err != nil {
			g.mu.Lock()
			g.removeNode(node)
			g.save()
			g.mu.Unlock()
			continue
		}
		// if connection succeeds, supply an unacceptable version to ensure
		// they won't try to add us as a peer
		encoding.WriteObject(conn, "0.0.0")
		conn.Close()
		// sleep for an extra 10 minutes after success; we don't want to spam
		// connectable nodes
		select {
		case <-time.After(10 * time.Minute):
		case <-g.threads.StopChan():
			return
		}
	}
}
