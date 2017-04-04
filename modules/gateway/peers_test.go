package gateway

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/fastrand"
	"github.com/NebulousLabs/muxado"
)

// dummyConn implements the net.Conn interface, but does not carry any actual
// data. It is passed to muxado, because passing nil results in segfaults.
type dummyConn struct {
	net.Conn
}

// muxado uses these methods when sending its GoAway signal
func (dc *dummyConn) Write(p []byte) (int, error) { return len(p), nil }

// Close is a no-op to satisfy the Conn interface.
func (dc *dummyConn) Close() error { return nil }

// SetWriteDeadline is a no-op to satisfy the Conn interface.
func (dc *dummyConn) SetWriteDeadline(time.Time) error { return nil }

// TestAddPeer tries adding a peer to the gateway.
func TestAddPeer(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newTestingGateway(t)
	defer g.Close()

	g.mu.Lock()
	defer g.mu.Unlock()
	g.addPeer(&peer{
		Peer: modules.Peer{
			NetAddress: "foo.com:123",
		},
		sess: muxado.Client(new(dummyConn)),
	})
	if len(g.peers) != 1 {
		t.Fatal("gateway did not add peer")
	}
}

// TestRandomInbountPeer checks that randomOutboundPeer returns the correct
// peer.
func TestRandomOutboundPeer(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newTestingGateway(t)
	defer g.Close()
	g.mu.Lock()
	defer g.mu.Unlock()

	_, err := g.randomOutboundPeer()
	if err != errNoPeers {
		t.Fatal("expected errNoPeers, got", err)
	}

	g.addPeer(&peer{
		Peer: modules.Peer{
			NetAddress: "foo.com:123",
			Inbound:    false,
		},
		sess: muxado.Client(new(dummyConn)),
	})
	if len(g.peers) != 1 {
		t.Fatal("gateway did not add peer")
	}
	addr, err := g.randomOutboundPeer()
	if err != nil || addr != "foo.com:123" {
		t.Fatal("gateway did not select random peer")
	}
}

// TestListen is a general test probling the connection listener.
func TestListen(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newTestingGateway(t)
	defer g.Close()

	// compliant connect with old version
	conn, err := net.Dial("tcp", string(g.Address()))
	if err != nil {
		t.Fatal("dial failed:", err)
	}
	addr := modules.NetAddress(conn.LocalAddr().String())
	ack, err := connectVersionHandshake(conn, "0.1")
	if err != errPeerRejectedConn {
		t.Fatal(err)
	}
	if ack != "" {
		t.Fatal("gateway should have rejected old version")
	}
	for i := 0; i < 10; i++ {
		g.mu.RLock()
		_, ok := g.peers[addr]
		g.mu.RUnlock()
		if ok {
			t.Fatal("gateway should not have added an old peer")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// a simple 'conn.Close' would not obey the muxado disconnect protocol
	muxado.Client(conn).Close()

	// compliant connect with invalid port
	conn, err = net.Dial("tcp", string(g.Address()))
	if err != nil {
		t.Fatal("dial failed:", err)
	}
	addr = modules.NetAddress(conn.LocalAddr().String())
	ack, err = connectVersionHandshake(conn, build.Version)
	if err != nil {
		t.Fatal(err)
	}
	if ack != build.Version {
		t.Fatal("gateway should have given ack")
	}
	err = connectPortHandshake(conn, "0")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		g.mu.RLock()
		_, ok := g.peers[addr]
		g.mu.RUnlock()
		if ok {
			t.Fatal("gateway should not have added a peer with an invalid port")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// a simple 'conn.Close' would not obey the muxado disconnect protocol
	muxado.Client(conn).Close()

	// compliant connect
	conn, err = net.Dial("tcp", string(g.Address()))
	if err != nil {
		t.Fatal("dial failed:", err)
	}
	addr = modules.NetAddress(conn.LocalAddr().String())
	ack, err = connectVersionHandshake(conn, build.Version)
	if err != nil {
		t.Fatal(err)
	}
	if ack != build.Version {
		t.Fatal("gateway should have given ack")
	}
	err = connectPortHandshake(conn, addr.Port())
	if err != nil {
		t.Fatal(err)
	}

	// g should add the peer
	var ok bool
	for !ok {
		g.mu.RLock()
		_, ok = g.peers[addr]
		g.mu.RUnlock()
	}

	muxado.Client(conn).Close()

	// g should remove the peer
	for ok {
		g.mu.RLock()
		_, ok = g.peers[addr]
		g.mu.RUnlock()
	}

	// uncompliant connect
	conn, err = net.Dial("tcp", string(g.Address()))
	if err != nil {
		t.Fatal("dial failed:", err)
	}
	if _, err := conn.Write([]byte("missing length prefix")); err != nil {
		t.Fatal("couldn't write malformed header")
	}
	// g should have closed the connection
	if n, err := conn.Write([]byte("closed")); err != nil && n > 0 {
		t.Error("write succeeded after closed connection")
	}
}

// TestConnect verifies that connecting peers will add peer relationships to
// the gateway, and that certain edge cases are properly handled.
func TestConnect(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create bootstrap peer
	bootstrap := newNamedTestingGateway(t, "1")
	defer bootstrap.Close()

	// give it a node
	bootstrap.mu.Lock()
	bootstrap.addNode(dummyNode)
	bootstrap.mu.Unlock()

	// create peer who will connect to bootstrap
	g := newNamedTestingGateway(t, "2")
	defer g.Close()

	// first simulate a "bad" connect, where bootstrap won't share its nodes
	bootstrap.mu.Lock()
	bootstrap.handlers[handlerName("ShareNodes")] = func(modules.PeerConn) error {
		return nil
	}
	bootstrap.mu.Unlock()
	// connect
	err := g.Connect(bootstrap.Address())
	if err != nil {
		t.Fatal(err)
	}
	// g should not have the node
	if g.removeNode(dummyNode) == nil {
		t.Fatal("bootstrapper should not have received dummyNode:", g.nodes)
	}

	// split 'em up
	g.Disconnect(bootstrap.Address())
	bootstrap.Disconnect(g.Address())

	// now restore the correct ShareNodes RPC and try again
	bootstrap.mu.Lock()
	bootstrap.handlers[handlerName("ShareNodes")] = bootstrap.shareNodes
	bootstrap.mu.Unlock()
	err = g.Connect(bootstrap.Address())
	if err != nil {
		t.Fatal(err)
	}
	// g should have the node
	time.Sleep(200 * time.Millisecond)
	g.mu.RLock()
	if _, ok := g.nodes[dummyNode]; !ok {
		g.mu.RUnlock() // Needed to prevent a deadlock if this error condition is reached.
		t.Fatal("bootstrapper should have received dummyNode:", g.nodes)
	}
	g.mu.RUnlock()
}

// TestUnitAcceptableVersion tests that the acceptableVersion func returns an
// error for unacceptable versions.
func TestUnitAcceptableVersion(t *testing.T) {
	invalidVersions := []string{
		// ascii gibberish
		"foobar",
		"foobar.0",
		"foobar.9",
		"0.foobar",
		"9.foobar",
		"foobar.0.0",
		"foobar.9.9",
		"0.foobar.0",
		"9.foobar.9",
		"0.0.foobar",
		"9.9.foobar",
		// utf-8 gibberish
		"世界",
		"世界.0",
		"世界.9",
		"0.世界",
		"9.世界",
		"世界.0.0",
		"世界.9.9",
		"0.世界.0",
		"9.世界.9",
		"0.0.世界",
		"9.9.世界",
		// missing numbers
		".",
		"..",
		"...",
		"0.",
		".1",
		"2..",
		".3.",
		"..4",
		"5.6.",
		".7.8",
		".9.0.",
	}
	for _, v := range invalidVersions {
		err := acceptableVersion(v)
		if _, ok := err.(invalidVersionError); err == nil || !ok {
			t.Errorf("acceptableVersion returned %q for version %q, but expected invalidVersionError", err, v)
		}
	}
	insufficientVersions := []string{
		// random small versions
		"0",
		"00",
		"0000000000",
		"0.0",
		"0000000000.0",
		"0.0000000000",
		"0.0.0.0.0.0.0.0",
		"0.0.9",
		"0.0.999",
		"0.0.99999999999",
		"0.1.2",
		"0.1.2.3.4.5.6.7.8.9",
		// pre-hardfork versions
		"0.3.3",
		"0.3.9.9.9.9.9.9.9.9.9.9",
		"0.3.9999999999",
	}
	for _, v := range insufficientVersions {
		err := acceptableVersion(v)
		if _, ok := err.(insufficientVersionError); err == nil || !ok {
			t.Errorf("acceptableVersion returned %q for version %q, but expected insufficientVersionError", err, v)
		}
	}
	validVersions := []string{
		minAcceptableVersion,
		"0.4.0",
		"0.6.0",
		"0.6.1",
		"0.9",
		"0.999",
		"0.9999999999",
		"1",
		"1.0",
		"1.0.0",
		"9",
		"9.0",
		"9.0.0",
		"9.9.9",
	}
	for _, v := range validVersions {
		err := acceptableVersion(v)
		if err != nil {
			t.Errorf("acceptableVersion returned %q for version %q, but expected nil", err, v)
		}
	}
}

// TestConnectRejectsInvalidAddrs tests that Connect only connects to valid IP
// addresses.
func TestConnectRejectsInvalidAddrs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newNamedTestingGateway(t, "1")
	defer g.Close()

	g2 := newNamedTestingGateway(t, "2")
	defer g2.Close()

	_, g2Port, err := net.SplitHostPort(string(g2.Address()))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		addr    modules.NetAddress
		wantErr bool
		msg     string
	}{
		{
			addr:    "127.0.0.1:123",
			wantErr: true,
			msg:     "Connect should reject unreachable addresses",
		},
		{
			addr:    "111.111.111.111:0",
			wantErr: true,
			msg:     "Connect should reject invalid NetAddresses",
		},
		{
			addr:    modules.NetAddress(net.JoinHostPort("localhost", g2Port)),
			wantErr: true,
			msg:     "Connect should reject non-IP addresses",
		},
		{
			addr: g2.Address(),
			msg:  "Connect failed to connect to another gateway",
		},
		{
			addr:    g2.Address(),
			wantErr: true,
			msg:     "Connect should reject an address it's already connected to",
		},
	}
	for _, tt := range tests {
		err := g.Connect(tt.addr)
		if tt.wantErr != (err != nil) {
			t.Errorf("%v, wantErr: %v, err: %v", tt.msg, tt.wantErr, err)
		}
	}
}

// TestConnectRejectsVersions tests that Gateway.Connect only accepts peers
// with sufficient and valid versions.
func TestConnectRejectsVersions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	g := newTestingGateway(t)
	defer g.Close()
	// Setup a listener that mocks Gateway.acceptConn, but sends the
	// version sent over mockVersionChan instead of build.Version.
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	tests := []struct {
		version             string
		errWant             error
		invalidVersion      bool
		insufficientVersion bool
		msg                 string
	}{
		// Test that Connect fails when the remote peer's version is "reject".
		{
			version: "reject",
			errWant: errPeerRejectedConn,
			msg:     "Connect should fail when the remote peer rejects the connection",
		},
		// Test that Connect fails when the remote peer's version is ascii gibberish.
		{
			version:        "foobar",
			invalidVersion: true,
			msg:            "Connect should fail when the remote peer's version is ascii gibberish",
		},
		// Test that Connect fails when the remote peer's version is utf8 gibberish.
		{
			version:        "世界",
			invalidVersion: true,
			msg:            "Connect should fail when the remote peer's version is utf8 gibberish",
		},
		// Test that Connect fails when the remote peer's version is < 0.4.0 (0).
		{
			version:             "0",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is 0",
		},
		{
			version:             "0.0.0",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is 0.0.0",
		},
		{
			version:             "0000.0000.0000",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is 0000.0000.0000",
		},
		{
			version:             "0.3.9",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is 0.3.9",
		},
		{
			version:             "0.3.9999",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is 0.3.9999",
		},
		{
			version:             "0.3.9.9.9",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is 0.3.9.9.9",
		},
		// Test that Connect succeeds when the remote peer's version is 0.4.0.
		{
			version: "0.4.0",
			msg:     "Connect should succeed when the remote peer's version is 0.4.0",
		},
		// Test that Connect succeeds when the remote peer's version is > 0.4.0.
		{
			version: "9",
			msg:     "Connect should succeed when the remote peer's version is 9",
		},
		{
			version: "9.9.9",
			msg:     "Connect should succeed when the remote peer's version is 9.9.9",
		},
		{
			version: "9999.9999.9999",
			msg:     "Connect should succeed when the remote peer's version is 9999.9999.9999",
		},
	}
	for _, tt := range tests {
		doneChan := make(chan struct{})
		go func() {
			defer close(doneChan)
			conn, err := listener.Accept()
			if err != nil {
				panic(err)
			}
			remoteVersion, err := acceptConnVersionHandshake(conn, tt.version)
			if err != nil {
				panic(err)
			}
			if remoteVersion != build.Version {
				panic("remoteVersion != build.Version")
			}
		}()
		err = g.Connect(modules.NetAddress(listener.Addr().String()))
		switch {
		case tt.invalidVersion:
			// Check that the error is the expected type.
			if _, ok := err.(invalidVersionError); !ok {
				t.Fatalf("expected Connect to error with invalidVersionError: %s", tt.msg)
			}
		case tt.insufficientVersion:
			// Check that the error is the expected type.
			if _, ok := err.(insufficientVersionError); !ok {
				t.Fatalf("expected Connect to error with insufficientVersionError: %s", tt.msg)
			}
		default:
			// Check that the error is the expected error.
			if err != tt.errWant {
				t.Fatalf("expected Connect to error with '%v', but got '%v': %s", tt.errWant, err, tt.msg)
			}
		}
		<-doneChan
		g.Disconnect(modules.NetAddress(listener.Addr().String()))
	}
}

// TestAcceptConnRejectsVersions tests that Gateway.acceptConn only accepts
// peers with sufficient and valid versions.
func TestAcceptConnRejectsVersions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newTestingGateway(t)
	defer g.Close()

	tests := []struct {
		remoteVersion       string
		versionResponseWant string
		errWant             error
		msg                 string
	}{
		// Test that acceptConn fails when the remote peer's version is "reject".
		{
			remoteVersion:       "reject",
			versionResponseWant: "",
			errWant:             errPeerRejectedConn,
			msg:                 "acceptConn shouldn't accept a remote peer whose version is \"reject\"",
		},
		// Test that acceptConn fails when the remote peer's version is ascii gibberish.
		{
			remoteVersion:       "foobar",
			versionResponseWant: "",
			errWant:             errPeerRejectedConn,
			msg:                 "acceptConn shouldn't accept a remote peer whose version is ascii gibberish",
		},
		// Test that acceptConn fails when the remote peer's version is utf8 gibberish.
		{
			remoteVersion:       "世界",
			versionResponseWant: "",
			errWant:             errPeerRejectedConn,
			msg:                 "acceptConn shouldn't accept a remote peer whose version is utf8 gibberish",
		},
		// Test that acceptConn fails when the remote peer's version is < 0.4.0 (0).
		{
			remoteVersion:       "0",
			versionResponseWant: "",
			errWant:             errPeerRejectedConn,
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0",
		},
		{
			remoteVersion:       "0.0.0",
			versionResponseWant: "",
			errWant:             errPeerRejectedConn,
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0.0.0",
		},
		{
			remoteVersion:       "0000.0000.0000",
			versionResponseWant: "",
			errWant:             errPeerRejectedConn,
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0000.000.000",
		},
		{
			remoteVersion:       "0.3.9",
			versionResponseWant: "",
			errWant:             errPeerRejectedConn,
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0.3.9",
		},
		{
			remoteVersion:       "0.3.9999",
			versionResponseWant: "",
			errWant:             errPeerRejectedConn,
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0.3.9999",
		},
		{
			remoteVersion:       "0.3.9.9.9",
			versionResponseWant: "",
			errWant:             errPeerRejectedConn,
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0.3.9.9.9",
		},
		// Test that acceptConn succeeds when the remote peer's version is 0.4.0.
		{
			remoteVersion:       "0.4.0",
			versionResponseWant: build.Version,
			msg:                 "acceptConn should accept a remote peer whose version is 0.4.0",
		},
		// Test that acceptConn succeeds when the remote peer's version is > 0.4.0.
		{
			remoteVersion:       "9",
			versionResponseWant: build.Version,
			msg:                 "acceptConn should accept a remote peer whose version is 9",
		},
		{
			remoteVersion:       "9.9.9",
			versionResponseWant: build.Version,
			msg:                 "acceptConn should accept a remote peer whose version is 9.9.9",
		},
		{
			remoteVersion:       "9999.9999.9999",
			versionResponseWant: build.Version,
			msg:                 "acceptConn should accept a remote peer whose version is 9999.9999.9999",
		},
	}
	for _, tt := range tests {
		conn, err := net.DialTimeout("tcp", string(g.Address()), dialTimeout)
		if err != nil {
			t.Fatal(err)
		}
		remoteVersion, err := connectVersionHandshake(conn, tt.remoteVersion)
		if err != tt.errWant {
			t.Fatal(err)
		}
		if remoteVersion != tt.versionResponseWant {
			t.Fatal(tt.msg)
		}
		conn.Close()
	}
}

// TestDisconnect checks that calls to gateway.Disconnect correctly disconnect
// and remove peers from the gateway.
func TestDisconnect(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g := newTestingGateway(t)
	defer g.Close()

	if err := g.Disconnect("bar.com:123"); err == nil {
		t.Fatal("disconnect removed unconnected peer")
	}

	// dummy listener to accept connection
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal("couldn't start listener:", err)
	}
	go func() {
		_, err := l.Accept()
		if err != nil {
			panic(err)
		}
	}()
	// skip standard connection protocol
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal("dial failed:", err)
	}
	g.mu.Lock()
	g.addPeer(&peer{
		Peer: modules.Peer{
			NetAddress: "foo.com:123",
		},
		sess: muxado.Client(conn),
	})
	g.mu.Unlock()
	if err := g.Disconnect("foo.com:123"); err != nil {
		t.Fatal("disconnect failed:", err)
	}
}

// TestPeerManager checks that the peer manager is properly spacing out peer
// connection requests.
func TestPeerManager(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	g1 := newNamedTestingGateway(t, "1")
	defer g1.Close()

	// create a valid node to connect to
	g2 := newNamedTestingGateway(t, "2")
	defer g2.Close()

	// g1's node list should only contain g2
	g1.mu.Lock()
	g1.nodes = map[modules.NetAddress]struct{}{}
	g1.nodes[g2.Address()] = struct{}{}
	g1.mu.Unlock()

	// when peerManager wakes up, it should connect to g2.
	time.Sleep(time.Second + noNodesDelay)

	g1.mu.RLock()
	defer g1.mu.RUnlock()
	if len(g1.peers) != 1 || g1.peers[g2.Address()] == nil {
		t.Fatal("gateway did not connect to g2:", g1.peers)
	}
}

// TestOverloadedBootstrap creates a bunch of gateways and connects all of them
// to the first gateway, the bootstrap gateway. More gateways will be created
// than is allowed by the bootstrap for the total number of connections. After
// waiting, all peers should eventually get to the full number of outbound
// peers.
func TestOverloadedBootstrap(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create fullyConnectedThreshold*2 peers and connect them all to only the
	// first node.
	var gs []*Gateway
	for i := 0; i < fullyConnectedThreshold*2; i++ {
		gs = append(gs, newNamedTestingGateway(t, strconv.Itoa(i)))
		// Connect this gateway to the first gateway.
		if i == 0 {
			continue
		}
		err := gs[i].Connect(gs[0].myAddr)
		for j := 0; j < 100 && err != nil; j++ {
			time.Sleep(time.Millisecond * 250)
			err = gs[i].Connect(gs[0].myAddr)
		}
		if err != nil {
			panic(err)
		}
	}

	// Spin until all gateways have a complete number of outbound peers.
	success := false
	for i := 0; i < 100; i++ {
		success = true
		for _, g := range gs {
			outboundPeers := 0
			g.mu.RLock()
			for _, p := range g.peers {
				if !p.Inbound {
					outboundPeers++
				}
			}
			g.mu.RUnlock()

			if outboundPeers < wellConnectedThreshold {
				success = false
				break
			}
		}
		if !success {
			time.Sleep(time.Second)
		}
	}
	if !success {
		for i, g := range gs {
			outboundPeers := 0
			g.mu.RLock()
			for _, p := range g.peers {
				if !p.Inbound {
					outboundPeers++
				}
			}
			g.mu.RUnlock()
			t.Log("Gateway", i, ":", outboundPeers)
		}
		t.Fatal("after 100 seconds not all gateways able to become well connected")
	}

	// Randomly close many of the peers. For many peers, this should put them
	// below the well connected threshold, but there are still enough nodes on
	// the network that no partitions should occur.
	var newGS []*Gateway
	for _, i := range fastrand.Perm(len(gs)) {
		newGS = append(newGS, gs[i])
	}
	cutSize := len(newGS) / 4
	// Close the first many of the now-randomly-sorted gateways.
	for _, g := range newGS[:cutSize] {
		err := g.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Set 'gs' equal to the remaining gateways.
	gs = newGS[cutSize:]

	// Spin until all gateways have a complete number of outbound peers. The
	// test can fail if there are network partitions, however not a huge
	// magnitude of nodes are being removed, and they all started with 4
	// connections. A partition is unlikely.
	success = false
	for i := 0; i < 100; i++ {
		success = true
		for _, g := range gs {
			outboundPeers := 0
			g.mu.RLock()
			for _, p := range g.peers {
				if !p.Inbound {
					outboundPeers++
				}
			}
			g.mu.RUnlock()

			if outboundPeers < wellConnectedThreshold {
				success = false
				break
			}
		}
		if !success {
			time.Sleep(time.Second)
		}
	}
	if !success {
		t.Fatal("after 100 seconds not all gateways able to become well connected")
	}

	// Close all remaining gateways.
	for _, g := range gs {
		err := g.Close()
		if err != nil {
			t.Error(err)
		}
	}
}
