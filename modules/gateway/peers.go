package gateway

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"

	"github.com/inconshreveable/muxado"
)

const dialTimeout = 10 * time.Second

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
	return &peerConn{conn, p.addr}, nil
}

func (p *peer) accept() (modules.PeerConn, error) {
	conn, err := p.sess.Accept()
	if err != nil {
		return nil, err
	}
	return &peerConn{conn, p.addr}, nil
}

// addPeer adds a peer to the Gateway's peer list and spawns a listener thread
// to handle its requests.
func (g *Gateway) addPeer(p *peer) {
	g.peers[p.addr] = p
	g.addNode(p.addr)
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
	var addr modules.NetAddress
	if err := encoding.ReadObject(conn, &addr, maxAddrLength); err != nil {
		conn.Close()
		return
	}
	g.log.Printf("INFO: %v wants to connect (gave address: %v)", conn.RemoteAddr(), addr)
	id := g.mu.Lock()
	g.addPeer(&peer{addr: addr, sess: muxado.Server(conn)})
	g.mu.Unlock(id)
	g.log.Println("INFO: accepted connection from new peer %v", addr)

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
	// send our address
	if err := encoding.WriteObject(conn, g.Address()); err != nil {
		return err
	}
	// TODO: exchange version messages

	id = g.mu.Lock()
	g.addPeer(&peer{addr: addr, sess: muxado.Client(conn)})
	g.mu.Unlock(id)

	g.log.Println("INFO: connected to new peer", addr)

	// request nodes
	nodes, err := g.requestNodes(addr)
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
			if err != nil || numPeers >= 8 {
				break
			}
			g.Connect(addr)
		}
		time.Sleep(5 * time.Second)
	}
}
