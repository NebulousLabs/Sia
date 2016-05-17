package gateway

import (
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/muxado"
)

const (
	// the gateway will abort a connection attempt after this long
	dialTimeout = 2 * time.Minute
	// the gateway will not accept inbound connections above this threshold
	fullyConnectedThreshold = 128
	// the gateway will ask for more addresses below this threshold
	minNodeListLen = 100
)

var (
	// The gateway will sleep this long between incoming connections.
	acceptInterval = func() time.Duration {
		switch build.Release {
		case "dev":
			return 3 * time.Second
		case "standard":
			return 3 * time.Second
		case "testing":
			return 10 * time.Millisecond
		default:
			panic("unrecognized build.Release")
		}
	}()

	errPeerRejectedConn = errors.New("peer rejected connection")
)

// insufficientVersionError indicates a peer's version is insufficient.
type insufficientVersionError string

// Error implements the error interface for insufficientVersionError.
func (s insufficientVersionError) Error() string {
	return "unacceptable version: " + string(s)
}

type peer struct {
	modules.Peer
	sess muxado.Session
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
	g.peers[p.NetAddress] = p
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
		for addr, p := range g.peers {
			// only select inbound peers
			if !p.Inbound {
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

		// Sleep after each accept. This limits the rate at which the Gateway
		// will accept new connections. The intent here is to prevent new
		// incoming connections from kicking out old ones before they have a
		// chance to request additional nodes.
		time.Sleep(acceptInterval)
	}
}

// acceptConn adds a connecting node as a peer.
func (g *Gateway) acceptConn(conn net.Conn) {
	addr := modules.NetAddress(conn.RemoteAddr().String())
	g.log.Debugf("INFO: %v wants to connect", addr)

	// read version
	var remoteVersion string
	if err := encoding.ReadObject(conn, &remoteVersion, build.MaxEncodedVersionLength); err != nil {
		conn.Close()
		g.log.Printf("INFO: %v wanted to connect, but we could not read their version: %v", addr, err)
		return
	}

	// Check that version is acceptable.
	//
	// Reject peers < v0.4.0 as the previous version is v0.3.3 which is
	// pre-hardfork.
	//
	// NOTE: this version must be bumped whenever the gateway or consensus
	// breaks compatibility.
	if !build.IsVersion(remoteVersion) || build.VersionCmp(remoteVersion, "0.4.0") < 0 {
		encoding.WriteObject(conn, "reject")
		conn.Close()
		g.log.Printf("INFO: %v wanted to connect, but their version (%v) was unacceptable", addr, remoteVersion)
		return
	}

	// respond with our version
	if err := encoding.WriteObject(conn, build.Version); err != nil {
		conn.Close()
		g.log.Printf("INFO: could not write version ack to %v: %v", addr, err)
		return
	}

	// Read the peer's port that we can dial them back on. Peers older than v0.6.1
	// will only be discovered by newer peers via the ShareNodes RPC.
	var remoteAddr modules.NetAddress
	if build.VersionCmp(remoteVersion, "0.6.1") >= 0 {
		var dialbackPort string
		err := encoding.ReadObject(conn, &dialbackPort, 13) // Max port # is 65535 (5 digits long) + 8 byte string length prefix
		if err != nil {
			conn.Close()
			g.log.Printf("INFO: could not read remote peer's (%v) port: %v", addr, err)
			return
		}
		if _, err := strconv.Atoi(dialbackPort); err != nil {
			conn.Close()
			g.log.Printf("INFO: peer (%v) sent an invalid dialback port: %v", addr, err)
			return
		}
		remoteAddr = modules.NetAddress(net.JoinHostPort(addr.Host(), dialbackPort))
		// This check should only fail if dialbackPort == 0, which we could check
		// manually, but there's no harm in validating the entire address.
		if err := remoteAddr.IsValid(); err != nil {
			conn.Close()
			g.log.Printf("INFO: peer's address (%v) is invalid: %v", remoteAddr, err)
			return
		}
		id := g.mu.Lock()
		_, exists := g.peers[remoteAddr]
		g.mu.Unlock(id)
		if exists {
			conn.Close()
			g.log.Printf("INFO: rejecting connection, already connected to a peer on that address: %v", remoteAddr)
			return
		}
	}

	// If we are already fully connected, kick out an old peer to make room
	// for the new one. Importantly, prioritize kicking a peer with the same
	// IP as the connecting peer. This protects against Sybil attacks.
	id := g.mu.Lock()
	if len(g.peers) >= fullyConnectedThreshold {
		// first choose a random peer, preferably inbound. If have only
		// outbound peers, we'll wind up kicking an outbound peer; but
		// subsequent inbound connections will kick each other instead of
		// continuing to replace outbound peers.
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
	p := peer{
		Peer: modules.Peer{
			NetAddress: addr,
			Inbound:    true,
			Version:    remoteVersion,
		},
		sess: muxado.Server(conn),
	}
	if build.VersionCmp(remoteVersion, "0.6.1") >= 0 {
		p.NetAddress = remoteAddr
	}
	g.addPeer(&p)
	// addNode must be called after addPeer so that threadedPeerManager doesn't
	// try to Connect to the node before we do (although in this particular case
	// it doesn't matter as a lock is held over both calls).
	var err error
	if build.VersionCmp(remoteVersion, "0.6.1") >= 0 {
		err = g.addNode(remoteAddr)
		// TODO: call g.save?
	}
	g.mu.Unlock(id)
	if err != nil && err != errNodeExists {
		g.Disconnect(remoteAddr)
		g.log.Debugf("INFO: %v wanted to connect, but we could not add node %q: %v", addr, remoteAddr, err)
		return
	}

	g.log.Debugf("INFO: accepted connection from new peer %v (v%v)", addr, remoteVersion)
}

// Connect establishes a persistent connection to a peer, and adds it to the
// Gateway's peer list.
func (g *Gateway) Connect(addr modules.NetAddress) error {
	if addr == g.Address() {
		return errors.New("can't connect to our own address")
	}
	if err := addr.IsValid(); err != nil {
		return errors.New("can't connect to invalid address")
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
	if err := encoding.WriteObject(conn, build.Version); err != nil {
		conn.Close()
		return err
	}
	// read version ack
	var remoteVersion string
	if err := encoding.ReadObject(conn, &remoteVersion, build.MaxEncodedVersionLength); err != nil {
		conn.Close()
		return err
	}
	// decide whether to accept this version
	if remoteVersion == "reject" {
		conn.Close()
		return errPeerRejectedConn
	}
	// Check that version is acceptable.
	//
	// Reject peers < v0.4.0 as the previous version is v0.3.3 which is
	// pre-hardfork.
	//
	// NOTE: this version must be bumped whenever the gateway or consensus
	// breaks compatibility.
	if !build.IsVersion(remoteVersion) || build.VersionCmp(remoteVersion, "0.4.0") < 0 {
		conn.Close()
		return insufficientVersionError(remoteVersion)
	}

	// Share our port with the peer so they can connect to us in the future.
	if build.VersionCmp(remoteVersion, "0.6.1") >= 0 {
		err := encoding.WriteObject(conn, g.port)
		if err != nil {
			conn.Close()
			return errors.New("could not write port #: " + err.Error())
		}
	}

	g.log.Debugln("INFO: connected to new peer", addr)

	id = g.mu.Lock()
	g.addPeer(&peer{
		Peer: modules.Peer{
			NetAddress: addr,
			Inbound:    false,
			Version:    remoteVersion,
		},
		sess: muxado.Client(conn),
	})
	// addNode must be called after addPeer so that threadedPeerManager doesn't
	// try to Connect to the node before we do (although in this particular case
	// it doesn't matter as a lock is held over both calls).
	err = g.addNode(addr)
	// TODO: call g.save?
	g.mu.Unlock(id)
	if err != nil && err != errNodeExists {
		g.Disconnect(addr)
		return err
	}

	// call initRPCs
	id = g.mu.RLock()
	for name, fn := range g.initRPCs {
		go func(name string, fn modules.RPCFunc) {
			err := g.RPC(addr, name, fn)
			if err != nil {
				g.log.Debugf("INFO: RPC %q on peer %q failed: %v", name, addr, err)
			}
		}(name, fn)
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
			if !p.Inbound {
				numOutboundPeers++
			}
		}
		addr, err := g.randomNode()
		g.mu.RUnlock(id)
		if numOutboundPeers >= modules.WellConnectedThreshold {
			select {
			case <-time.After(5 * time.Minute):
			case <-g.closeChan:
				return
			}
			continue
		}

		// Try to connect to a random node. Instead of blocking on Connect, we
		// spawn a goroutine and sleep for five seconds. This allows us to
		// continue making connections if the node is unresponsive.
		if err == nil {
			go g.Connect(addr)
		}
		select {
		case <-time.After(5 * time.Second):
		case <-g.closeChan:
			return
		}
	}
}

// Peers returns the addresses currently connected to the Gateway.
func (g *Gateway) Peers() []modules.Peer {
	id := g.mu.RLock()
	defer g.mu.RUnlock(id)
	var peers []modules.Peer
	for _, p := range g.peers {
		peers = append(peers, p.Peer)
	}
	return peers
}
