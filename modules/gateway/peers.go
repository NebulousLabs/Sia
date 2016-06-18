package gateway

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/muxado"
)

const (
	// the gateway will not accept inbound connections above this threshold
	fullyConnectedThreshold = 128

	// the gateway will ask for more addresses below this threshold
	minNodeListLen = 100

	// minAcceptableVersion is the version below which the gateway will refuse to
	// connect to peers and reject connection attempts.
	//
	// Reject peers < v0.4.0 as the previous version is v0.3.3 which is
	// pre-hardfork.
	minAcceptableVersion = "0.4.0"
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
	// the gateway will abort a connection attempt after this long
	dialTimeout = func() time.Duration {
		switch build.Release {
		case "dev":
			return 2 * time.Minute
		case "standard":
			return 2 * time.Minute
		case "testing":
			return 2 * time.Second
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

// invalidVersionError indicates a peer's version is not a valid version number.
type invalidVersionError string

// Error implements the error interface for invalidVersionError.
func (s invalidVersionError) Error() string {
	return "invalid version: " + string(s)
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
	go g.threadedListenPeer(p)
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

// threadedListen handles incoming connection requests. If the connection is accepted,
// the peer will be added to the Gateway's peer list.
func (g *Gateway) threadedListen() {
	if g.threads.Add() != nil {
		return
	}
	defer g.threads.Done()

	for {
		conn, err := g.listener.Accept()
		if err != nil {
			return
		}

		go g.threadedAcceptConn(conn)

		// Sleep after each accept. This limits the rate at which the Gateway
		// will accept new connections. The intent here is to prevent new
		// incoming connections from kicking out old ones before they have a
		// chance to request additional nodes.
		select {
		case <-time.After(acceptInterval):
		case <-g.threads.StopChan():
			return
		}
	}
}

// threadedAcceptConn adds a connecting node as a peer.
func (g *Gateway) threadedAcceptConn(conn net.Conn) {
	if g.threads.Add() != nil {
		return
	}
	defer g.threads.Done()

	addr := modules.NetAddress(conn.RemoteAddr().String())
	g.log.Printf("INFO: %v wants to connect", addr)

	remoteVersion, err := acceptConnVersionHandshake(conn, build.Version)
	if err != nil {
		g.log.Debugf("INFO: %v wanted to connect but version handshake failed: %v", addr, err)
		// TODO: should this be muxado.Server(conn).Close()?
		conn.Close()
		return
	}

	// If we are already fully connected, kick out an old peer to make room
	// for the new one. Importantly, prioritize kicking a peer with the same
	// IP as the connecting peer. This protects against Sybil attacks.
	g.mu.Lock()
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
	g.addPeer(&peer{
		Peer: modules.Peer{
			NetAddress: addr,
			Inbound:    true,
			Version:    remoteVersion,
		},
		sess: muxado.Server(conn),
	})
	g.mu.Unlock()

	g.log.Printf("INFO: accepted connection from new peer %v (v%v)", addr, remoteVersion)
}

// acceptableVersion returns an error if the version is unacceptable.
func acceptableVersion(version string) error {
	if !build.IsVersion(version) {
		return invalidVersionError(version)
	}
	if build.VersionCmp(version, minAcceptableVersion) < 0 {
		return insufficientVersionError(version)
	}
	return nil
}

// connectVersionHandshake performs the version handshake and should be called
// on the side making the connection request. The remote version is only
// returned if err == nil.
func connectVersionHandshake(conn net.Conn, version string) (remoteVersion string, err error) {
	// Send our version.
	if err := encoding.WriteObject(conn, version); err != nil {
		return "", fmt.Errorf("failed to write version: %v", err)
	}
	// Read remote version.
	if err := encoding.ReadObject(conn, &remoteVersion, maxAddrLength); err != nil {
		return "", fmt.Errorf("failed to read remote version: %v", err)
	}
	// Check that their version is acceptable.
	if remoteVersion == "reject" {
		return "", errPeerRejectedConn
	}
	if err := acceptableVersion(remoteVersion); err != nil {
		return "", err
	}
	return remoteVersion, nil
}

// acceptConnVersionHandshake performs the version handshake and should be
// called on the side accepting a connection request. The remote version is
// only returned in err == nil.
func acceptConnVersionHandshake(conn net.Conn, version string) (remoteVersion string, err error) {
	// Read remote version.
	if err := encoding.ReadObject(conn, &remoteVersion, maxAddrLength); err != nil {
		return "", fmt.Errorf("failed to read remote version: %v", err)
	}
	// Check that their version is acceptable.
	if err := acceptableVersion(remoteVersion); err != nil {
		if err := encoding.WriteObject(conn, "reject"); err != nil {
			return "", fmt.Errorf("failed to write reject: %v", err)
		}
		return "", err
	}
	// Send our version.
	if err := encoding.WriteObject(conn, version); err != nil {
		return "", fmt.Errorf("failed to write version: %v", err)
	}
	return remoteVersion, nil
}

// Connect establishes a persistent connection to a peer, and adds it to the
// Gateway's peer list.
func (g *Gateway) Connect(addr modules.NetAddress) error {
	if err := g.threads.Add(); err != nil {
		return err
	}
	defer g.threads.Done()

	if addr == g.Address() {
		return errors.New("can't connect to our own address")
	}
	if err := addr.IsValid(); err != nil {
		return errors.New("can't connect to invalid address")
	}

	g.mu.RLock()
	_, exists := g.peers[addr]
	g.mu.RUnlock()
	if exists {
		return errors.New("peer already added")
	}

	conn, err := net.DialTimeout("tcp", string(addr), dialTimeout)
	if err != nil {
		return err
	}
	remoteVersion, err := connectVersionHandshake(conn, build.Version)
	if err != nil {
		// TODO: should this be muxado.Client(conn).Close()?
		conn.Close()
		return err
	}

	g.log.Println("INFO: connected to new peer", addr)

	g.mu.Lock()
	g.addPeer(&peer{
		Peer: modules.Peer{
			NetAddress: addr,
			Inbound:    false,
			Version:    remoteVersion,
		},
		sess: muxado.Client(conn),
	})
	g.mu.Unlock()

	// call initRPCs
	g.mu.RLock()
	for name, fn := range g.initRPCs {
		go func(name string, fn modules.RPCFunc) {
			if g.threads.Add() != nil {
				return
			}
			defer g.threads.Done()

			g.RPC(addr, name, fn)
		}(name, fn)
	}
	g.mu.RUnlock()

	return nil
}

// Disconnect terminates a connection to a peer and removes it from the
// Gateway's peer list. The peer's address remains in the node list.
func (g *Gateway) Disconnect(addr modules.NetAddress) error {
	if err := g.threads.Add(); err != nil {
		return err
	}
	defer g.threads.Done()

	g.mu.RLock()
	p, exists := g.peers[addr]
	g.mu.RUnlock()
	if !exists {
		return errors.New("not connected to that node")
	}
	g.mu.Lock()
	delete(g.peers, addr)
	g.mu.Unlock()
	if err := p.sess.Close(); err != nil {
		return err
	}

	g.log.Println("INFO: disconnected from peer", addr)
	return nil
}

// threadedPeerManager tries to keep the Gateway well-connected. As long as
// the Gateway is not well-connected, it tries to connect to random nodes.
func (g *Gateway) threadedPeerManager() {
	if g.threads.Add() != nil {
		return
	}
	defer g.threads.Done()

	for {
		// If we are well-connected, sleep in increments of five minutes until
		// we are no longer well-connected.
		g.mu.RLock()
		numOutboundPeers := 0
		for _, p := range g.peers {
			if !p.Inbound {
				numOutboundPeers++
			}
		}
		addr, err := g.randomNode()
		g.mu.RUnlock()
		if numOutboundPeers >= modules.WellConnectedThreshold {
			select {
			case <-time.After(5 * time.Minute):
			case <-g.threads.StopChan():
				return
			}
			continue
		}

		// Try to connect to a random node. Instead of blocking on Connect, we
		// spawn a goroutine and sleep for five seconds. This allows us to
		// continue making connections if the node is unresponsive.
		if err == nil {
			go func() {
				if g.threads.Add() != nil {
					return
				}
				defer g.threads.Done()

				connectErr := g.Connect(addr)
				if connectErr != nil {
					g.log.Debugln("WARN: automatic connect failed:", connectErr)
				}
			}()
		}
		select {
		case <-time.After(5 * time.Second):
		case <-g.threads.StopChan():
			return
		}
	}
}

// Peers returns the addresses currently connected to the Gateway.
func (g *Gateway) Peers() []modules.Peer {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var peers []modules.Peer
	for _, p := range g.peers {
		peers = append(peers, p.Peer)
	}
	return peers
}
