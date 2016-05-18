package gateway

import (
	"net"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
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
	g := newTestingGateway("TestAddPeer", t)
	defer g.Close()
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
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
	g := newTestingGateway("TestRandomInboundPeer", t)
	defer g.Close()
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
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
	// send version
	if err := encoding.WriteObject(conn, "0.1"); err != nil {
		t.Fatal("couldn't write version")
	}
	// read ack
	var ack string
	if err := encoding.ReadObject(conn, &ack, build.MaxEncodedVersionLength); err != nil {
		t.Fatal(err)
	} else if ack != "reject" {
		t.Fatal("gateway should have rejected old version")
	}
	// g should not add the peer
	time.Sleep(200 * time.Millisecond)
	id := g.mu.RLock()
	_, ok := g.peers[addr]
	g.mu.RUnlock(id)
	if ok {
		t.Fatal("gateway should not have added a peer with too old of a version")
	}

	// a simple 'conn.Close' would not obey the muxado disconnect protocol
	muxado.Client(conn).Close()

	// compliant connect
	conn, err = net.Dial("tcp", string(g.Address()))
	if err != nil {
		t.Fatal("dial failed:", err)
	}
	addr = modules.NetAddress(conn.LocalAddr().String())
	// send version
	if err := encoding.WriteObject(conn, build.Version); err != nil {
		t.Fatal("couldn't write version")
	}
	// read ack
	if err := encoding.ReadObject(conn, &ack, build.MaxEncodedVersionLength); err != nil {
		t.Fatal(err)
	} else if ack == "reject" {
		t.Fatal("gateway should have given ack")
	}
	// send port
	if err := encoding.WriteObject(conn, addr.Port()); err != nil {
		t.Fatal(err)
	}

	// g should add the peer
	time.Sleep(200 * time.Millisecond)
	id = g.mu.RLock()
	_, ok = g.peers[addr]
	g.mu.RUnlock(id)
	if !ok {
		t.Fatal("gateway should have added the peer")
	}

	muxado.Client(conn).Close()

	// g should remove the peer
	time.Sleep(200 * time.Millisecond)
	id = g.mu.RLock()
	_, ok = g.peers[addr]
	g.mu.RUnlock(id)
	if ok {
		t.Fatal("gateway should have removed the peer")
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
	id := bootstrap.mu.Lock()
	bootstrap.addNode(dummyNode)
	bootstrap.mu.Unlock(id)

	// create peer who will connect to bootstrap
	g := newTestingGateway("TestConnect2", t)
	defer g.Close()

	// first simulate a "bad" connect, where bootstrap won't share its nodes
	bootstrap.handlers[handlerName("ShareNodes")] = func(modules.PeerConn) error {
		return nil
	}
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
	bootstrap.handlers[handlerName("ShareNodes")] = bootstrap.shareNodes
	err = g.Connect(bootstrap.Address())
	if err != nil {
		t.Fatal(err)
	}
	// g should have the node
	time.Sleep(100 * time.Millisecond)
	id = g.mu.RLock()
	if _, ok := g.nodes[dummyNode]; !ok {
		t.Fatal("bootstrapper should have received dummyNode:", g.nodes)
	}
	g.mu.RUnlock(id)
}

// TestConnectRejectsInvalidAddrs tests that Connect only connects to valid IP
// addresses.
func TestConnectRejectsInvalidAddrs(t *testing.T) {
	g := newTestingGateway("TestConnectRejectsInvalidAddrs", t)
	defer g.Close()

	g2 := newTestingGateway("TestConnectRejectsInvalidAddrs2", t)
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

// TestConnectRejectsInvalidVersions tests that Gateway.Connect only accepts
// peers with sufficient and valid versions.
func TestConnectRejectsInvalidVersions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	g := newTestingGateway("TestConnectRejectsInvalidVersions", t)
	defer g.Close()
	// Setup a listener that mocks Gateway.acceptConn, but sends the
	// version sent over mockVersionChan instead of build.Version.
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	mockVersionChan := make(chan string)
	go func() {
		for {
			mockVersion := <-mockVersionChan
			conn, err := listener.Accept()
			if err != nil {
				// Panic because t.Fatal doesn't work from goroutines.
				panic(err)
			}
			// Read remote peer version.
			var remoteVersion string
			if err := encoding.ReadObject(conn, &remoteVersion, build.MaxEncodedVersionLength); err != nil {
				panic(err)
			}
			// Write our mock version.
			if err := encoding.WriteObject(conn, mockVersion); err != nil {
				panic(err)
			}
			if build.IsVersion(mockVersion) && build.VersionCmp(mockVersion, "0.6.1") >= 0 {
				var port string
				if err := encoding.ReadObject(conn, &port, 13); err != nil {
					panic(err)
				}
			}
		}
	}()

	tests := []struct {
		version             string
		errWant             error
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
			version:             "foobar",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is ascii gibberish",
		},
		{
			version:             "999.foobar",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is ascii gibberish",
		},
		{
			version:             "foobar.0",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is ascii gibberish",
		},
		// Test that Connect fails when the remote peer's version is utf8 gibberish.
		{
			version:             "世界",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is utf8 gibberish",
		},
		{
			version:             "999.世界",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is utf8 gibberish",
		},
		{
			version:             "世界.0",
			insufficientVersion: true,
			msg:                 "Connect should fail when the remote peer's version is utf8 gibberish",
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
		mockVersionChan <- tt.version
		err = g.Connect(modules.NetAddress(listener.Addr().String()))
		if tt.insufficientVersion {
			// Check that the error is the expected type.
			if _, ok := err.(insufficientVersionError); !ok {
				t.Fatalf("expected Connect to error with insufficientVersionError: %s", tt.msg)
			}
		} else {
			// Check that the error is the expected error.
			if err != tt.errWant {
				t.Fatalf("expected Connect to error with '%v', but got '%v': %s", tt.errWant, err, tt.msg)
			}
		}
		g.Disconnect(modules.NetAddress(listener.Addr().String()))
	}
	listener.Close()
}

// mockGatewayWithVersion is a mock implementation of Gateway that sends a mock
// version on Connect instead of build.Version.
type mockGatewayWithVersion struct {
	*Gateway
	version    string
	versionACK chan string
}

// Connect is a mock implementation of modules.Gateway.Connect that provides a
// mock version to peers it connects to instead of build.Version. The version
// ack written by the remote peer is written to the versionACK channel.
func (g mockGatewayWithVersion) Connect(addr modules.NetAddress) error {
	conn, err := net.DialTimeout("tcp", string(addr), dialTimeout)
	if err != nil {
		return err
	}
	// send mocked version
	if err := encoding.WriteObject(conn, g.version); err != nil {
		return err
	}
	// read version ack
	var remoteVersion string
	if err := encoding.ReadObject(conn, &remoteVersion, build.MaxEncodedVersionLength); err != nil {
		return err
	}
	// send port
	if remoteVersion != "reject" && build.IsVersion(g.version) && build.VersionCmp(g.version, "0.6.1") >= 0 && build.VersionCmp(remoteVersion, "0.6.1") >= 0 {
		if err := encoding.WriteObject(conn, g.port); err != nil {
			return err
		}
	}
	g.versionACK <- remoteVersion

	return nil
}

// TestAcceptConnRejectsInvalidVersions tests that Gateway.acceptConn only
// accepts peers with sufficient and valid versions.
func TestAcceptConnRejectsInvalidVersions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	g := newTestingGateway("TestAcceptConnRejectsInvalidVersions1", t)
	defer g.Close()
	mg := mockGatewayWithVersion{
		Gateway:    newTestingGateway("TestAcceptConnRejectsInvalidVersions2", t),
		versionACK: make(chan string),
	}
	defer mg.Close()

	tests := []struct {
		remoteVersion       string
		versionResponseWant string
		msg                 string
	}{
		// Test that acceptConn fails when the remote peer's version is "reject".
		{
			remoteVersion:       "reject",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is \"reject\"",
		},
		// Test that acceptConn fails when the remote peer's version is ascii gibberish.
		{
			remoteVersion:       "foobar",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is ascii giberish",
		},
		{
			remoteVersion:       "999.foobar",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is ascii giberish",
		},
		{
			remoteVersion:       "foobar.0",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is ascii giberish",
		},
		// Test that acceptConn fails when the remote peer's version is utf8 gibberish.
		{
			remoteVersion:       "世界",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is utf8 giberish",
		},
		{
			remoteVersion:       "999.世界",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is utf8 giberish",
		},
		{
			remoteVersion:       "世界.0",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is utf8 giberish",
		},
		// Test that acceptConn fails when the remote peer's version is < 0.4.0 (0).
		{
			remoteVersion:       "0",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0",
		},
		{
			remoteVersion:       "0.0.0",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0.0.0",
		},
		{
			remoteVersion:       "0000.0000.0000",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0000.000.000",
		},
		{
			remoteVersion:       "0.3.9",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0.3.9",
		},
		{
			remoteVersion:       "0.3.9999",
			versionResponseWant: "reject",
			msg:                 "acceptConn shouldn't accept a remote peer whose version is 0.3.9999",
		},
		{
			remoteVersion:       "0.3.9.9.9",
			versionResponseWant: "reject",
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
		// g should not be connected to any peers.
		id := g.mu.Lock()
		if len(g.peers) != 0 {
			t.Errorf("Gateway should not be connected to any peers: %v", g.peers)
		}
		g.mu.Unlock(id)
		// Connect mg to g.
		mg.version = tt.remoteVersion
		go func() {
			err := mg.Connect(g.Address())
			if err != nil {
				// Panic because t.Fatal doesn't work from goroutines.
				panic(err)
			}
		}()
		// Check that the version response is correct and that the gateway's
		// connected if it was.
		remoteVersion := <-mg.versionACK
		if remoteVersion != tt.versionResponseWant {
			t.Fatalf(tt.msg)
		}
		if remoteVersion != "reject" {
			time.Sleep(200 * time.Millisecond)
			id := g.mu.Lock()
			if len(g.peers) != 1 {
				t.Errorf("Gateway should be connected to only the one peer, but it is connected to: %v", g.peers)
			}
			g.mu.Unlock(id)
		}
		// Disconnect
		// We can't do g.Disconnect(mg.Address()) because the identifying address for
		// mg might be a random port if it's version # is < v0.6.1.
		id = g.mu.Lock()
		var mgAddress modules.NetAddress
		for mgAddress = range g.peers {
			break
		}
		g.mu.Unlock(id)
		g.Disconnect(mgAddress)
	}
}

func TestDisconnect(t *testing.T) {
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
	id := g.mu.Lock()
	g.addPeer(&peer{
		Peer: modules.Peer{
			NetAddress: "foo.com:123",
		},
		sess: muxado.Client(conn),
	})
	g.mu.Unlock(id)
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
	id := g1.mu.Lock()
	g1.nodes = map[modules.NetAddress]struct{}{}
	g1.nodes[g2.Address()] = struct{}{}
	g1.mu.Unlock(id)

	// when peerManager wakes up, it should connect to g2.
	time.Sleep(6 * time.Second)

	id = g1.mu.RLock()
	defer g1.mu.RUnlock(id)
	if len(g1.peers) != 1 || g1.peers[g2.Address()] == nil {
		t.Fatal("gateway did not connect to g2:", g1.peers)
	}
}
