package gateway

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"

	"github.com/inconshreveable/muxado"
)

const (
	dialTimeout = 2 * time.Minute
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
		r, _ := crypto.RandIntn(len(g.peers))
		for addr := range g.peers {
			if r <= 0 {
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
		r, _ := crypto.RandIntn(len(g.peers))
		for addr, peer := range g.peers {
			// only select inbound peers
			if !peer.inbound {
				continue
			}
			if r <= 0 {
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

	// read version
	var remoteVersion string
	if err := encoding.ReadObject(conn, &remoteVersion, maxAddrLength); err != nil {
		conn.Close()
		g.log.Printf("INFO: %v wanted to connect, but we could not read their version: %v", addr, err)
		return
	}

	// check that version is acceptable
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

	// If we are already fully connected, kick out an old peer to make room
	// for the new one. Importantly, prioritize kicking a peer with the same
	// IP as the connecting peer. This protects against Sybil attacks.
	id := g.mu.Lock()
	if len(g.peers) >= fullyConnectedThreshold {
		// first choose a random peer, preferably inbound
		kick, err := g.randomInboundPeer()
		if err != nil {
			kick, _ = g.randomPeer()
		}
		// if another peer shares this IP, choose that one instead
		for p := range g.peers {
			if p.Host() == addr.Host() {
				kick = p
				break
			}
		}
		g.peers[kick].sess.Close()
		delete(g.peers, kick)
		g.log.Printf("INFO: disconnected from %v to make room for %v", kick, addr)
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
	for name, fn := range g.initRPCs {
		go g.RPC(addr, name, fn)
	}
	g.mu.RUnlock(id)

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

// threadedPeerManager tries to keep the Gateway well-connected. As long as
// the Gateway is not well-connected, it tries to connect to random nodes.
func (g *Gateway) threadedPeerManager() {
	for {
		// If we are well-connected, sleep in increments of five minutes until
		// we are no longer well-connected.
		id := g.mu.RLock()
		numOutboundPeers := 0
		for _, p := range g.peers {
			if !p.inbound {
				numOutboundPeers++
			}
		}
		addr, err := g.randomNode()
		g.mu.RUnlock(id)
		if numOutboundPeers >= wellConnectedThreshold {
			time.Sleep(5 * time.Minute)
			continue
		}

		// Try to connect to a random node. Instead of blocking on Connect, we
		// spawn a goroutine and sleep for five seconds. This allows us to
		// continue making connections if the node is unresponsive.
		if err == nil {
			go g.Connect(addr)
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
