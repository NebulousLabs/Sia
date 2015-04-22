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

type Peer struct {
	sess    muxado.Session
	strikes int
}

// addPeer adds a peer to the Gateway's peer list and spawns a listener thread
// to handle its requests.
func (g *Gateway) addPeer(conn net.Conn, addr modules.NetAddress) *Peer {
	peer := &Peer{muxado.Server(conn), 0}
	id := g.mu.Lock()
	g.peers[addr] = peer
	g.addNode(addr)
	g.mu.Unlock(id)
	go g.listenPeer(peer)
	return peer
}

// connect establishes a persistent connection to a peer, and adds it to the
// Gateway's peer list.
func (g *Gateway) Connect(addr modules.NetAddress) (*Peer, error) {
	if addr == g.myAddr {
		return nil, errors.New("can't connect to our own address")
	}

	id := g.mu.RLock()
	_, exists := g.peers[addr]
	g.mu.RUnlock(id)
	if exists {
		return nil, errors.New("peer already added")
	}

	conn, err := net.DialTimeout("tcp", string(addr), dialTimeout)
	if err != nil {
		return nil, err
	}
	// send our address
	if err := encoding.WriteObject(conn, g.myAddr); err != nil {
		return nil, err
	}
	// TODO: exchange version messages

	peer := g.addPeer(conn, addr)

	g.log.Println("INFO: connected to new peer", addr)
	return peer, nil
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
				return
			}
			g.log.Printf("INFO: %v wants to connect (gave address: %v)\n", conn.RemoteAddr(), addr)
			g.addPeer(conn, addr)
		}(conn)
	}
}
