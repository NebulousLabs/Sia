package gateway

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"

	"github.com/inconshreveable/muxado"
)

const (
	version = "0.1"

	dialTimeout = 10 * time.Second
	// the gateway will not make outbound connections above this threshold
	wellConnectedThreshold = 8
	// the gateway will not accept inbound connections above this threshold
	fullyConnectedThreshold = 128
)

type peer struct {
	strikes uint32
	addr    modules.NetAddress
	sess    muxado.Session
}

func (p *peer) open() (modules.PeerConn, error) {
	conn, err := p.sess.Open()
	if err != nil {
		return nil, err
	}
	return &peerConn{conn}, nil
}

func (p *peer) accept() (modules.PeerConn, error) {
	conn, err := p.sess.Accept()
	if err != nil {
		return nil, err
	}
	return &peerConn{conn}, nil
}

// addPeer adds a peer to the Gateway's peer list and spawns a listener thread
// to handle its requests.
func (g *Gateway) addPeer(p *peer) {
	g.peers[p.addr] = p
	go g.listenPeer(p)
}

// listen handles incoming connection requests. If the connection is accepted,
// the peer will be added to the Gateway's peer list.
func (g *Gateway) listen() {
	for {
		conn, err := g.listener.Accept()
		if err != nil {
			return
		}

		go g.acceptConn(conn)
	}
}

// acceptConn adds a connecting node as a peer.
// TODO: reject when we have too many active connections
func (g *Gateway) acceptConn(conn net.Conn) {
	g.log.Printf("INFO: %v wants to connect", conn.RemoteAddr())

	// read version
	var remoteVersion string
	if err := encoding.ReadObject(conn, &remoteVersion, maxAddrLength); err != nil {
		conn.Close()
		g.log.Printf("INFO: %v wanted to connect, but we could not read their version: %v", conn.RemoteAddr(), err)
		return
	}

	// decide whether to accept
	id := g.mu.RLock()
	numPeers := len(g.peers)
	g.mu.RUnlock(id)
	if numPeers >= fullyConnectedThreshold {
		encoding.WriteObject(conn, "reject")
		conn.Close()
		g.log.Printf("INFO: rejected connection from %v (already have %v peers)", conn.RemoteAddr(), len(g.peers))
		return
	}
	// TODO: reject old versions

	// send ack
	if err := encoding.WriteObject(conn, "accept"); err != nil {
		conn.Close()
		g.log.Printf("INFO: could not write ack to %v: %v", conn.RemoteAddr(), err)
		return
	}

	// add the peer
	id = g.mu.Lock()
	g.addPeer(&peer{addr: modules.NetAddress(conn.RemoteAddr().String()), sess: muxado.Server(conn)})
	g.mu.Unlock(id)
	g.log.Printf("INFO: accepted connection from new peer %v (v%v)", conn.RemoteAddr(), remoteVersion)
}

// Connect establishes a persistent connection to a peer, and adds it to the
// Gateway's peer list.
func (g *Gateway) Connect(addr modules.NetAddress) error {
	if addr == g.Address() {
		return errors.New("can't connect to our own address")
	}

	id := g.mu.RLock()
	_, exists := g.peers[addr]
	g.mu.RUnlock(id)
	if exists {
		return errors.New("peer already added")
	}

	conn, err := net.DialTimeout("tcp", string(addr), dialTimeout)
	if err != nil {
		return err
	}
	// send our version
	if err := encoding.WriteObject(conn, version); err != nil {
		return err
	}
	// read ack
	var ack string
	if err := encoding.ReadObject(conn, &ack, maxAddrLength); err != nil {
		return err
	} else if ack != "accept" {
		return errors.New("peer rejected connection")
	}

	g.log.Println("INFO: connected to new peer", addr)

	id = g.mu.Lock()
	g.addPeer(&peer{addr: addr, sess: muxado.Client(conn)})
	g.mu.Unlock(id)

	// Tell the peer to add our callback address as a node
	err = g.RPC(addr, "RelayNode", func(conn modules.PeerConn) error {
		return encoding.WriteObject(conn, g.Address())
	})
	if err != nil {
		// log this error, but don't return it
		g.log.Printf("WARN: could not relay our address to %v: %v", addr, err)
	}

	// request nodes
	var nodes []modules.NetAddress
	err = g.RPC(addr, "ShareNodes", func(conn modules.PeerConn) error {
		return encoding.ReadObject(conn, &nodes, maxSharedNodes*maxAddrLength)
	})
	if err != nil {
		// log this error, but don't return it
		g.log.Printf("WARN: request for node list of %v failed: %v", addr, err)
		return nil
	}
	g.log.Printf("INFO: %v sent us %v peers", addr, len(nodes))
	id = g.mu.Lock()
	for _, node := range nodes {
		g.addNode(node)
	}
	g.save()
	g.mu.Unlock(id)

	return nil
}

// Disconnect terminates a connection to a peer and removes it from the
// Gateway's peer list. The peer's address remains in the node list.
func (g *Gateway) Disconnect(addr modules.NetAddress) error {
	id := g.mu.RLock()
	p, exists := g.peers[addr]
	g.mu.RUnlock(id)
	if !exists {
		return errors.New("not connected to that node")
	}
	p.sess.Close()
	id = g.mu.Lock()
	delete(g.peers, addr)
	g.mu.Unlock(id)

	g.log.Println("INFO: disconnected from peer", addr)
	return nil
}

// makeOutboundConnections tries to keep the Gateway well-connected. As long
// as the Gateway is not well-connected, it tries to add random nodes as
// peers. It sleeps when the Gateway becomes well-connected, or it has tried
// more than 100 nodes.
func (g *Gateway) makeOutboundConnections() {
	for {
		for i := 0; i < 100; i++ {
			id := g.mu.RLock()
			numPeers := len(g.peers)
			addr, err := g.randomNode()
			g.mu.RUnlock(id)
			if err != nil || numPeers >= wellConnectedThreshold {
				break
			}
			g.Connect(addr)
		}
		time.Sleep(5 * time.Second)
	}
}
