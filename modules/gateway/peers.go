package gateway

import (
	"errors"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"

	"github.com/inconshreveable/muxado"
)

const (
	dialTimeout = 10 * time.Second
	// the gateway will not make outbound connections above this threshold
	wellConnectedThreshold = 8
	// the gateway will not accept inbound connections above this threshold
	fullyConnectedThreshold = 128
	// the gateway will ask for more addresses below this threshold
	minNodeListLen = 100
)

type peer struct {
	addr    modules.NetAddress
	sess    muxado.Session
	inbound bool
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

// randomPeer returns a random peer from the gateway's peer list.
func (g *Gateway) randomPeer() (modules.NetAddress, error) {
	if len(g.peers) > 0 {
		r := rand.Intn(len(g.peers))
		for addr := range g.peers {
			if r == 0 {
				return addr, nil
			}
			r--
		}
	}

	return "", errNoPeers
}

// randomInboundPeer returns a random peer that initiated its connection.
func (g *Gateway) randomInboundPeer() (modules.NetAddress, error) {
	if len(g.peers) > 0 {
		r := rand.Intn(len(g.peers))
		for addr, peer := range g.peers {
			// only select inbound peers
			if !peer.inbound {
				continue
			}
			if r == 0 {
				return addr, nil
			}
			r--
		}
	}

	return "", errNoPeers
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
func (g *Gateway) acceptConn(conn net.Conn) {
	addr := modules.NetAddress(conn.RemoteAddr().String())
	g.log.Printf("INFO: %v wants to connect", addr)

	// don't connect to an IP address more than once
	if build.Release != "testing" {
		id := g.mu.RLock()
		for p := range g.peers {
			if p.Host() == addr.Host() {
				g.mu.RUnlock(id)
				conn.Close()
				g.log.Printf("INFO: rejected connection from %v: already connected", addr)
				return
			}
		}
		g.mu.RUnlock(id)
	}

	// read version
	var remoteVersion string
	if err := encoding.ReadObject(conn, &remoteVersion, maxAddrLength); err != nil {
		conn.Close()
		g.log.Printf("INFO: %v wanted to connect, but we could not read their version: %v", addr, err)
		return
	}

	// decide whether to accept
	// NOTE: this version must be bumped whenever the gateway or consensus
	// breaks compatibility.
	if build.VersionCmp(remoteVersion, "0.3.3") < 0 {
		encoding.WriteObject(conn, "reject")
		conn.Close()
		g.log.Printf("INFO: %v wanted to connect, but their version (%v) was unacceptable", addr, remoteVersion)
		return
	}

	// respond with our version
	if err := encoding.WriteObject(conn, "0.3.3"); err != nil {
		conn.Close()
		g.log.Printf("INFO: could not write version ack to %v: %v", addr, err)
		return
	}

	// If we are already fully connected, kick out an old inbound peer to make
	// room for the new one. Among other things, this ensures that bootstrap
	// nodes will always be connectible. Worst case, you'll connect, receive a
	// node list, and immediately get booted. But once you have the node list
	// you should be able to connect to less full peers.
	id := g.mu.Lock()
	if len(g.peers) >= fullyConnectedThreshold {
		oldPeer, err := g.randomInboundPeer()
		if err == nil {
			g.peers[oldPeer].sess.Close()
			delete(g.peers, oldPeer)
			g.log.Printf("INFO: disconnected from %v to make room for %v", oldPeer, addr)
		}
	}
	// add the peer
	g.addPeer(&peer{addr: addr, sess: muxado.Server(conn), inbound: true})
	g.mu.Unlock(id)

	g.log.Printf("INFO: accepted connection from new peer %v (v%v)", addr, remoteVersion)
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
	if err := encoding.WriteObject(conn, "0.3.3"); err != nil {
		return err
	}
	// read version ack
	var remoteVersion string
	if err := encoding.ReadObject(conn, &remoteVersion, maxAddrLength); err != nil {
		return err
	} else if remoteVersion == "reject" {
		return errors.New("peer rejected connection")
	}
	// decide whether to accept this version
	if build.VersionCmp(remoteVersion, "0.3.3") < 0 {
		conn.Close()
		return errors.New("unacceptable version: " + remoteVersion)
	}

	g.log.Println("INFO: connected to new peer", addr)

	id = g.mu.Lock()
	g.addPeer(&peer{addr: addr, sess: muxado.Client(conn), inbound: false})
	g.mu.Unlock(id)

	// call initRPCs
	id = g.mu.RLock()
	var wg sync.WaitGroup
	wg.Add(len(g.initRPCs))
	for name, fn := range g.initRPCs {
		go func(name string, fn modules.RPCFunc) {
			// errors here are non-fatal
			g.RPC(addr, name, fn)
			wg.Done()
		}(name, fn)
	}
	g.mu.RUnlock(id)
	wg.Wait()

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
			err = g.Connect(addr)
			// aggressively remove unresponsive nodes
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				id = g.mu.Lock()
				g.removeNode(addr)
				g.mu.Unlock(id)
			}
		}
		// request more nodes if necessary
		id := g.mu.RLock()
		numNodes := len(g.nodes)
		addr, err := g.randomPeer()
		g.mu.RUnlock(id)
		if build.Release != "testing" && err == nil && numNodes < minNodeListLen {
			g.RPC(addr, "ShareNodes", g.requestNodes)
		}
		time.Sleep(5 * time.Second)
	}
}

func (g *Gateway) Peers() []modules.NetAddress {
	id := g.mu.RLock()
	defer g.mu.RUnlock(id)
	var peers []modules.NetAddress
	for addr := range g.peers {
		peers = append(peers, addr)
	}
	return peers
}
