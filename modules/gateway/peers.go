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
	sess    muxado.Session
}

// addPeer adds a peer to the Gateway's peer list and spawns a listener thread
// to handle its requests.
func (g *Gateway) addPeer(addr modules.NetAddress, p *peer) {
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	g.peers[addr] = p
	g.addNode(addr)
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
		// for now just accept all requests
		// TODO: reject when we have too many active connections
		go func(conn net.Conn) {
			var addr modules.NetAddress
			if err := encoding.ReadObject(conn, &addr, maxAddrLength); err != nil {
				conn.Close()
				return
			}
			g.log.Printf("INFO: %v wants to connect (gave address: %v)", conn.RemoteAddr(), addr)
			g.addPeer(addr, &peer{sess: muxado.Server(conn)})
		}(conn)
	}
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

	g.addPeer(addr, &peer{sess: muxado.Client(conn)})

	g.log.Println("INFO: connected to new peer", addr)
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
