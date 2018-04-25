package gateway

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

var (
	errNodeExists    = errors.New("node already added")
	errNoNodes       = errors.New("no nodes in the node list")
	errOurAddress    = errors.New("can't add our own address")
	errPeerGenesisID = errors.New("peer has different genesis ID")
)

// A node represents a potential peer on the Sia network.
type node struct {
	NetAddress      modules.NetAddress `json:"netaddress"`
	WasOutboundPeer bool               `json:"wasoutboundpeer"`
}

// addNode adds an address to the set of nodes on the network.
func (g *Gateway) addNode(addr modules.NetAddress) error {
	if addr == g.myAddr {
		return errOurAddress
	} else if _, exists := g.nodes[addr]; exists {
		return errNodeExists
	} else if addr.IsStdValid() != nil {
		return errors.New("address is not valid: " + string(addr))
	} else if net.ParseIP(addr.Host()) == nil {
		return errors.New("address must be an IP address: " + string(addr))
	}
	g.nodes[addr] = &node{
		NetAddress:      addr,
		WasOutboundPeer: false,
	}
	return nil
}

// staticPingNode verifies that there is a reachable node at the provided address
// by performing the Sia gateway handshake protocol.
func (g *Gateway) staticPingNode(addr modules.NetAddress) error {
	// Ping the untrusted node to see whether or not there's actually a
	// reachable node at the provided address.
	conn, err := g.staticDial(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Read the node's version.
	remoteVersion, err := connectVersionHandshake(conn, build.Version)
	if err != nil {
		return err
	}

	if build.VersionCmp(remoteVersion, minimumAcceptablePeerVersion) < 0 {
		return nil // for older versions, this is where pinging ends
	}

	// Send our header.
	// NOTE: since we don't intend to complete the connection, we can send an
	// inaccurate NetAddress.
	ourHeader := sessionHeader{
		GenesisID:  types.GenesisID,
		UniqueID:   g.staticId,
		NetAddress: modules.NetAddress(conn.LocalAddr().String()),
	}
	if err := exchangeOurHeader(conn, ourHeader); err != nil {
		return err
	}

	// Read remote header.
	var remoteHeader sessionHeader
	if err := encoding.ReadObject(conn, &remoteHeader, maxEncodedSessionHeaderSize); err != nil {
		return fmt.Errorf("failed to read remote header: %v", err)
	} else if err := acceptableSessionHeader(ourHeader, remoteHeader, conn.RemoteAddr().String()); err != nil {
		return err
	}

	// Send special rejection string.
	if err := encoding.WriteObject(conn, modules.StopResponse); err != nil {
		return fmt.Errorf("failed to write header rejection: %v", err)
	}
	return nil
}

// removeNode will remove a node from the gateway.
func (g *Gateway) removeNode(addr modules.NetAddress) error {
	if _, exists := g.nodes[addr]; !exists {
		return errors.New("no record of that node")
	}
	delete(g.nodes, addr)
	return nil
}

// randomNode returns a random node from the gateway. An error can be returned
// if there are no nodes in the node list.
func (g *Gateway) randomNode() (modules.NetAddress, error) {
	if len(g.nodes) == 0 {
		return "", errNoPeers
	}

	// Select a random peer. Note that the algorithm below is roughly linear in
	// the number of nodes known by the gateway, and this number can approach
	// every node on the network. If the network gets large, this algorithm
	// will either need to be refactored, or more likely a cap on the size of
	// g.nodes will need to be added.
	r := fastrand.Intn(len(g.nodes))
	for node := range g.nodes {
		if r <= 0 {
			return node, nil
		}
		r--
	}
	return "", errNoPeers
}

// shareNodes is the receiving end of the ShareNodes RPC. It writes up to 10
// randomly selected nodes to the caller.
func (g *Gateway) shareNodes(conn modules.PeerConn) error {
	conn.SetDeadline(time.Now().Add(connStdDeadline))
	remoteNA := modules.NetAddress(conn.RemoteAddr().String())

	// Assemble a list of nodes to send to the peer.
	var nodes []modules.NetAddress
	func() {
		g.mu.RLock()
		defer g.mu.RUnlock()

		// Gather candidates for sharing.
		gnodes := make([]modules.NetAddress, 0, len(g.nodes))
		for node := range g.nodes {
			// Don't share local peers with remote peers. That means that if 'node'
			// is loopback, it will only be shared if the remote peer is also
			// loopback. And if 'node' is private, it will only be shared if the
			// remote peer is either the loopback or is also private.
			if node.IsLoopback() && !remoteNA.IsLoopback() {
				continue
			}
			if node.IsLocal() && !remoteNA.IsLocal() {
				continue
			}
			gnodes = append(gnodes, node)
		}

		// Iterate through the random permutation of nodes and select the
		// desirable ones.
		for _, i := range fastrand.Perm(len(gnodes)) {
			nodes = append(nodes, gnodes[i])
			if uint64(len(nodes)) == maxSharedNodes {
				break
			}
		}
	}()
	return encoding.WriteObject(conn, nodes)
}

// requestNodes is the calling end of the ShareNodes RPC.
func (g *Gateway) requestNodes(conn modules.PeerConn) error {
	conn.SetDeadline(time.Now().Add(connStdDeadline))

	var nodes []modules.NetAddress
	if err := encoding.ReadObject(conn, &nodes, maxSharedNodes*modules.MaxEncodedNetAddressLength); err != nil {
		return err
	}

	g.mu.Lock()
	changed := false
	for _, node := range nodes {
		err := g.addNode(node)
		if err != nil && err != errNodeExists && err != errOurAddress {
			g.log.Printf("WARN: peer '%v' sent the invalid addr '%v'", conn.RPCAddr(), node)
		}
		if err == nil {
			changed = true
		}
	}
	if changed {
		err := g.saveSync()
		if err != nil {
			g.log.Println("ERROR: unable to save new nodes added to the gateway:", err)
		}
	}
	g.mu.Unlock()
	return nil
}

// permanentNodePurger is a thread that runs throughout the lifetime of the
// gateway, purging unconnectable nodes from the node list in a sustainable
// way.
func (g *Gateway) permanentNodePurger(closeChan chan struct{}) {
	defer close(closeChan)

	for {
		// Choose an amount of time to wait before attempting to prune a node.
		// Nodes will occasionally go offline for some time, which can even be
		// days. We don't want to too aggressively prune nodes with low-moderate
		// uptime, as they are still useful to the network.
		//
		// But if there are a lot of nodes, we want to make sure that the node
		// list does not become saturated with inaccessible / offline nodes.
		// Pruning happens a lot faster when there are a lot of nodes in the
		// gateway.
		//
		// This value is a ratelimit which tries to keep the nodes list in the
		// gateawy healthy. A more complex algorithm might adjust this number
		// according to the percentage of prune attempts that are successful
		// (decrease prune frequency if most nodes in the database are online,
		// increase prune frequency if more nodes in the database are offline).
		waitTime := nodePurgeDelay
		g.mu.RLock()
		nodeCount := len(g.nodes)
		g.mu.RUnlock()
		if nodeCount > quickPruneListLen {
			waitTime = fastNodePurgeDelay
		}

		// Sleep as a purge ratelimit.
		select {
		case <-time.After(waitTime):
		case <-g.threads.StopChan():
			// The gateway is shutting down, close out the thread.
			return
		}

		// Get a random node for scanning.
		g.mu.RLock()
		numNodes := len(g.nodes)
		node, err := g.randomNode()
		g.mu.RUnlock()
		if err == errNoNodes {
			// errNoNodes is a common error that will be resolved by the
			// bootstrap process.
			continue
		} else if err != nil {
			// Unusual error, create a logging statement.
			g.log.Println("ERROR: could not pick a random node for uptime check:", err)
			continue
		}
		if numNodes <= pruneNodeListLen {
			// There are not enough nodes in the gateway - pruning more is
			// probably a bad idea, and may affect the user's ability to
			// connect to the network in the future.
			continue
		}
		// Check whether this node is already a peer. If so, no need to dial
		// them.
		g.mu.RLock()
		_, exists := g.peers[node]
		g.mu.RUnlock()
		if exists {
			continue
		}

		// Try connecting to the random node. If the node is not reachable,
		// remove them from the node list.
		//
		// NOTE: an error may be returned if the dial is canceled partway
		// through, which would cause the node to be pruned even though it may
		// be a good node. Because nodes are plentiful, this is an acceptable
		// bug.
		if err = g.staticPingNode(node); err != nil {
			g.mu.Lock()
			if len(g.nodes) > pruneNodeListLen {
				// Check if the number of nodes is still above the threshold.
				g.removeNode(node)
				g.log.Debugf("INFO: removing node %q because it could not be reached during a random scan: %v", node, err)
			}
			g.mu.Unlock()
		}
	}
}

// permanentNodeManager tries to keep the Gateway's node list healthy. As long
// as the Gateway has fewer than healthyNodeListLen nodes, it asks a random
// peer for more nodes. It also continually pings nodes in order to establish
// their connectivity. Unresponsive nodes are aggressively removed.
func (g *Gateway) permanentNodeManager(closeChan chan struct{}) {
	defer close(closeChan)

	for {
		// Wait 5 seconds so that a controlled number of node requests are made
		// to peers.
		select {
		case <-time.After(nodeListDelay):
		case <-g.threads.StopChan():
			// Gateway is shutting down, close the thread.
			return
		}

		g.mu.RLock()
		numNodes := len(g.nodes)
		peer, err := g.randomOutboundPeer()
		g.mu.RUnlock()
		if err == errNoPeers {
			// errNoPeers is a common and expected error, there's no need to
			// log it.
			continue
		} else if err != nil {
			g.log.Println("ERROR: could not fetch a random peer:", err)
			continue
		}

		// Determine whether there are a satisfactory number of nodes in the
		// nodelist. If there are not, use the random peer from earlier to
		// expand the node list.
		if numNodes < healthyNodeListLen {
			err := g.managedRPC(peer, "ShareNodes", g.requestNodes)
			if err != nil {
				g.log.Debugf("WARN: RPC ShareNodes failed on peer %q: %v", peer, err)
				continue
			}
		} else {
			// There are enough nodes in the gateway, no need to check for more
			// every 5 seconds. Wait a while before checking again.
			select {
			case <-time.After(wellConnectedDelay):
			case <-g.threads.StopChan():
				// Gateway is shutting down, close the thread.
				return
			}
		}
	}
}
