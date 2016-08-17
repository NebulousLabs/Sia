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
	// fullyConnectedThreshold defines the number of peers that the gateway can
	// have before it stops accepting inbound connections.
	fullyConnectedThreshold = 128

	// pruneNodeListLen defines the number of nodes that the gateway must have
	// to be pruning nodes from the node list.
	pruneNodeListLen = 50

	// minNodeListLen defines the number of nodes that the gateway must have in
	// the node list before it will stop asking peers for more nodes.
	minNodeListLen = 200

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
	return &peerConn{conn, p.NetAddress}, nil
}

func (p *peer) accept() (modules.PeerConn, error) {
	conn, err := p.sess.Accept()
	if err != nil {
		return nil, err
	}
	return &peerConn{conn, p.NetAddress}, nil
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

// permanentListen handles incoming connection requests. If the connection is
// accepted, the peer will be added to the Gateway's peer list.
func (g *Gateway) permanentListen(closeChan chan struct{}) {
	// Signal that the permanentListen thread has completed upon returning.
	defer close(closeChan)

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
		conn.Close()
		return
	}
	defer g.threads.Done()

	addr := modules.NetAddress(conn.RemoteAddr().String())
	g.log.Debugf("INFO: %v wants to connect", addr)

	remoteVersion, err := acceptConnVersionHandshake(conn, build.Version)
	if err != nil {
		g.log.Debugf("INFO: %v wanted to connect but version handshake failed: %v", addr, err)
		conn.Close()
		return
	}

	if build.VersionCmp(remoteVersion, "1.0.0") < 0 {
		err = g.managedAcceptConnOldPeer(conn, remoteVersion)
	} else {
		err = g.managedAcceptConnNewPeer(conn, remoteVersion)
	}
	if err != nil {
		g.log.Debugf("INFO: %v wanted to connect, but failed: %v", addr, err)
		conn.Close()
		return
	}

	g.log.Debugf("INFO: accepted connection from new peer %v (v%v)", addr, remoteVersion)
}

// managedAcceptConnOldPeer accepts a connection request from peers < v1.0.0.
// The requesting peer is added as a peer, but is not added to the node list
// (older peers do not share their dialback address). The peer is only added if
// a nil error is returned.
func (g *Gateway) managedAcceptConnOldPeer(conn net.Conn, remoteVersion string) error {
	addr := modules.NetAddress(conn.RemoteAddr().String())

	g.mu.Lock()
	defer g.mu.Unlock()

	g.acceptPeer(&peer{
		Peer: modules.Peer{
			NetAddress: addr,
			Inbound:    true,
			Version:    remoteVersion,
		},
		sess: muxado.Server(conn),
	})

	return nil
}

// managedAcceptConnNewPeer accepts connection requests from peers >= v1.0.0.
// The requesting peer is added as a node and a peer. The peer is only added if
// a nil error is returned.
func (g *Gateway) managedAcceptConnNewPeer(conn net.Conn, remoteVersion string) error {
	// Learn the peer's dialback address. Peers older than v1.0.0 will only be
	// able to be discovered by newer peers via the ShareNodes RPC.
	remoteAddr, err := acceptConnPortHandshake(conn)
	if err != nil {
		return err
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Don't accept a connection from a peer we're already connected to.
	if _, exists := g.peers[remoteAddr]; exists {
		return fmt.Errorf("already connected to a peer on that address: %v", remoteAddr)
	}

	err = g.addNode(remoteAddr)
	if err != nil && err != errNodeExists {
		return fmt.Errorf("error adding node %q: %v", remoteAddr, err)
	}
	err = g.save()
	if err != nil {
		return fmt.Errorf("error saving node list: %v", err)
	}

	g.acceptPeer(&peer{
		Peer: modules.Peer{
			NetAddress: remoteAddr,
			Inbound:    true,
			Version:    remoteVersion,
		},
		sess: muxado.Server(conn),
	})

	return nil
}

// acceptPeer makes room for the peer if necessary by kicking out existing
// peers, then adds the peer to the peer list.
func (g *Gateway) acceptPeer(p *peer) {
	// If we are already fully connected, kick out an old peer to make room
	// for the new one. Importantly, prioritize kicking a peer with the same
	// IP as the connecting peer. This protects against Sybil attacks.
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
		for addr := range g.peers {
			if addr.Host() == p.NetAddress.Host() {
				kick = addr
				break
			}
		}
		g.peers[kick].sess.Close()
		delete(g.peers, kick)
		g.log.Printf("INFO: disconnected from %v to make room for %v", kick, p.NetAddress)
	}

	g.addPeer(p)
}

// acceptConnPortHandshake performs the port handshake and should be called on
// the side accepting a connection request. The remote address is only returned
// if err == nil.
func acceptConnPortHandshake(conn net.Conn) (remoteAddr modules.NetAddress, err error) {
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return "", err
	}

	// Read the peer's port that we can dial them back on.
	var dialbackPort string
	err = encoding.ReadObject(conn, &dialbackPort, 13) // Max port # is 65535 (5 digits long) + 8 byte string length prefix
	if err != nil {
		return "", fmt.Errorf("could not read remote peer's port: %v", err)
	}
	remoteAddr = modules.NetAddress(net.JoinHostPort(host, dialbackPort))
	if err := remoteAddr.IsValid(); err != nil {
		return "", fmt.Errorf("peer's address (%v) is invalid: %v", remoteAddr, err)
	}
	// Sanity check to ensure that appending the port string to the host didn't
	// change the host. Only necessary because the peer sends the port as a string
	// instead of an integer.
	if remoteAddr.Host() != host {
		return "", fmt.Errorf("peer sent a port which modified the host")
	}
	return remoteAddr, nil
}

// connectPortHandshake performs the port handshake and should be called on the
// side initiating the connection request. This shares our port with the peer
// so they can connect to us in the future.
func connectPortHandshake(conn net.Conn, port string) error {
	err := encoding.WriteObject(conn, port)
	if err != nil {
		return errors.New("could not write port #: " + err.Error())
	}
	return nil
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
	if err := encoding.ReadObject(conn, &remoteVersion, build.MaxEncodedVersionLength); err != nil {
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
// only returned if err == nil.
func acceptConnVersionHandshake(conn net.Conn, version string) (remoteVersion string, err error) {
	// Read remote version.
	if err := encoding.ReadObject(conn, &remoteVersion, build.MaxEncodedVersionLength); err != nil {
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

// managedConnectOldPeer connects to peers < v1.0.0. The peer is added as a
// node and a peer. The peer is only added if a nil error is returned.
func (g *Gateway) managedConnectOldPeer(conn net.Conn, remoteVersion string, remoteAddr modules.NetAddress) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	err := g.addNode(remoteAddr)
	if err != nil && err != errNodeExists {
		return err
	}
	err = g.save()
	if err != nil {
		return fmt.Errorf("error saving node list: %v", err)
	}

	g.addPeer(&peer{
		Peer: modules.Peer{
			NetAddress: remoteAddr,
			Inbound:    false,
			Version:    remoteVersion,
		},
		sess: muxado.Client(conn),
	})
	return nil
}

// managedConnectNewPeer connects to peers >= v1.0.0. The peer is added as a
// node and a peer. The peer is only added if a nil error is returned.
func (g *Gateway) managedConnectNewPeer(conn net.Conn, remoteVersion string, remoteAddr modules.NetAddress) error {
	g.mu.RLock()
	port := g.port
	g.mu.RUnlock()
	// Send our dialable address to the peer so they can dial us back should we
	// disconnect.
	err := connectPortHandshake(conn, port)
	if err != nil {
		return err
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	err = g.addNode(remoteAddr)
	if err != nil && err != errNodeExists {
		return err
	}
	err = g.save()
	if err != nil {
		return fmt.Errorf("error saving node list: %v", err)
	}

	g.addPeer(&peer{
		Peer: modules.Peer{
			NetAddress: remoteAddr,
			Inbound:    false,
			Version:    remoteVersion,
		},
		sess: muxado.Client(conn),
	})
	return nil
}

// managedConnect establishes a persistent connection to a peer, and adds it to
// the Gateway's peer list.
func (g *Gateway) managedConnect(addr modules.NetAddress) error {
	if addr == g.Address() {
		return errors.New("can't connect to our own address")
	}
	if err := addr.IsValid(); err != nil {
		return errors.New("can't connect to invalid address")
	}
	if net.ParseIP(addr.Host()) == nil {
		return errors.New("address must be an IP address")
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
		conn.Close()
		return err
	}

	if build.VersionCmp(remoteVersion, "1.0.0") < 0 {
		err = g.managedConnectOldPeer(conn, remoteVersion, addr)
	} else {
		err = g.managedConnectNewPeer(conn, remoteVersion, addr)
	}
	if err != nil {
		conn.Close()
		return err
	}

	g.log.Debugln("INFO: connected to new peer", addr)

	// call initRPCs
	g.mu.RLock()
	for name, fn := range g.initRPCs {
		go func(name string, fn modules.RPCFunc) {
			if g.threads.Add() != nil {
				return
			}
			defer g.threads.Done()

			err := g.managedRPC(addr, name, fn)
			if err != nil {
				g.log.Debugf("INFO: RPC %q on peer %q failed: %v", name, addr, err)
			}
		}(name, fn)
	}
	g.mu.RUnlock()

	return nil
}

// Connect establishes a persistent connection to a peer, and adds it to the
// Gateway's peer list.
func (g *Gateway) Connect(addr modules.NetAddress) error {
	if err := g.threads.Add(); err != nil {
		return err
	}
	defer g.threads.Done()
	return g.managedConnect(addr)
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

// permanentPeerManager tries to keep the Gateway well-connected. As long as
// the Gateway is not well-connected, it tries to connect to random nodes.
func (g *Gateway) permanentPeerManager(closedChan chan struct{}) {
	// Send a signal upon shutdown.
	defer close(closedChan)

	for {
		// Determine the number of outbound peers.
		numOutboundPeers := 0
		g.mu.RLock()
		for _, p := range g.peers {
			if !p.Inbound {
				numOutboundPeers++
			}
		}
		g.mu.RUnlock()
		// If the gateway is well connected, sleep for 5 minutes and then check
		// again.
		if numOutboundPeers >= modules.WellConnectedThreshold {
			select {
			case <-time.After(5 * time.Minute):
			case <-g.threads.StopChan():
				// Interrupt the thread if the shutdown signal is issued.
				return
			}
			continue
		}

		// Fetch a random peer.
		g.mu.RLock()
		addr, err := g.randomNode()
		g.mu.RUnlock()
		// If there was an error, log the error and then wait a while before
		// trying again.
		if err != nil {
			select {
			case <-time.After(time.Second * 20):
			case <-g.threads.StopChan():
				// Interrupt the thread if the shutdown signal is issued.
				return
			}
			continue
		}

		// Try connecting to that peer in a goroutine.
		go func() {
			if err := g.threads.Add(); err != nil {
				return
			}
			defer g.threads.Done()

			err := g.managedConnect(addr)
			if err != nil {
				g.log.Debugln("WARN: automatic connect failed:", err)
			}
		}()

		// Wait a bit before trying the next peer. The peer connections are
		// non-blocking, so they should be spaced out to avoid spinning up an
		// uncontrolled number of threads and therefore peer connections.
		select {
		case <-time.After(5 * time.Second):
		case <-g.threads.StopChan():
			// Interrupt the thread if the shutdown signal is issued.
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
