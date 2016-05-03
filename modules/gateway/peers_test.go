package gateway

import (
	"net"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/muxado"
)

// dummyConn implements the net.Conn interface, but does not carry any actual
// data. It is passed to muxado, because passing nil results in segfaults.
type dummyConn struct {
	net.Conn
}

// muxado uses these methods when sending its GoAway signal
func (dc *dummyConn) Write(p []byte) (int, error) { return len(p), nil }

func (dc *dummyConn) Close() error { return nil }

func (dc *dummyConn) SetWriteDeadline(time.Time) error { return nil }

func TestAddPeer(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g := newTestingGateway("TestAddPeer", t)
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

func TestRandomInboundPeer(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g := newTestingGateway("TestRandomInboundPeer", t)
	defer g.Close()
	g.mu.Lock()
	defer g.mu.Unlock()
	_, err := g.randomInboundPeer()
	if err != errNoPeers {
		t.Fatal("expected errNoPeers, got", err)
	}

	g.addPeer(&peer{
		Peer: modules.Peer{
			NetAddress: "foo.com:123",
			Inbound:    true,
		},
		sess: muxado.Client(new(dummyConn)),
	})
	if len(g.peers) != 1 {
		t.Fatal("gateway did not add peer")
	}
	addr, err := g.randomInboundPeer()
	if err != nil || addr != "foo.com:123" {
		t.Fatal("gateway did not select random peer")
	}
}

func TestListen(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g := newTestingGateway("TestListen", t)
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

func TestConnect(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// create bootstrap peer
	bootstrap := newTestingGateway("TestConnect1", t)
	defer bootstrap.Close()

	// give it a node
	bootstrap.mu.Lock()
	bootstrap.addNode(dummyNode)
	bootstrap.mu.Unlock()

	// create peer who will connect to bootstrap
	g := newTestingGateway("TestConnect2", t)
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
	time.Sleep(100 * time.Millisecond)
	g.mu.RLock()
	if _, ok := g.nodes[dummyNode]; !ok {
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
	g := newTestingGateway("TestConnectRejectsInvalidAddrs", t)
	g2 := newTestingGateway("TestConnectRejectsInvalidAddrs2", t)
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
	g := newTestingGateway("TestConnectRejectsVersions", t)
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
	g := newTestingGateway("TestAcceptConnRejectsVersions", t)
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

func TestDisconnect(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g := newTestingGateway("TestDisconnect", t)
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
			t.Fatal("accept failed:", err)
		}
		// conn.Close()
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

func TestPeerManager(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g1 := newTestingGateway("TestPeerManager1", t)
	defer g1.Close()

	// create a valid node to connect to
	g2 := newTestingGateway("TestPeerManager2", t)
	defer g2.Close()

	// g1's node list should only contain g2
	g1.mu.Lock()
	g1.nodes = map[modules.NetAddress]struct{}{}
	g1.nodes[g2.Address()] = struct{}{}
	g1.mu.Unlock()

	// when peerManager wakes up, it should connect to g2.
	time.Sleep(6 * time.Second)

	g1.mu.RLock()
	defer g1.mu.RUnlock()
	if len(g1.peers) != 1 || g1.peers[g2.Address()] == nil {
		t.Fatal("gateway did not connect to g2:", g1.peers)
	}
}
