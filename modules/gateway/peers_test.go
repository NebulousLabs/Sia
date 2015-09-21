package gateway

import (
	"net"
	"testing"
	"time"

	"github.com/inconshreveable/muxado"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

func TestAddPeer(t *testing.T) {
	g := newTestingGateway("TestAddPeer", t)
	defer g.Close()
	id := g.mu.Lock()
	defer g.mu.Unlock(id)
	g.addPeer(&peer{addr: "foo", sess: muxado.Client(nil)})
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

	g.addPeer(&peer{addr: "foo", sess: muxado.Client(nil), inbound: true})
	if len(g.peers) != 1 {
		t.Fatal("gateway did not add peer")
	}
	addr, err := g.randomInboundPeer()
	if err != nil || addr != "foo" {
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
	if err := encoding.ReadObject(conn, &ack, maxAddrLength); err != nil {
		t.Fatal(err)
	} else if ack != "reject" {
		t.Fatal("gateway should have rejected old version")
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
	if err := encoding.ReadObject(conn, &ack, maxAddrLength); err != nil {
		t.Fatal(err)
	} else if ack == "reject" {
		t.Fatal("gateway should have given ack")
	}

	// g should add the peer
	var ok bool
	for !ok {
		id := g.mu.RLock()
		_, ok = g.peers[addr]
		g.mu.RUnlock(id)
	}

	muxado.Client(conn).Close()

	// g should remove the peer
	for ok {
		id := g.mu.RLock()
		_, ok = g.peers[addr]
		g.mu.RUnlock(id)
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
	bootstrap.RegisterRPC("ShareNodes", func(modules.PeerConn) error {
		return nil
	})
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
	bootstrap.RegisterRPC("ShareNodes", bootstrap.shareNodes)
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

func TestDisconnect(t *testing.T) {
	g := newTestingGateway("TestDisconnect", t)
	defer g.Close()

	if err := g.Disconnect("bar"); err == nil {
		t.Fatal("disconnect removed unconnected peer")
	}

	// dummy listener to accept connection
	l, err := net.Listen("tcp", ":0")
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
	g.addPeer(&peer{addr: "foo", sess: muxado.Client(conn)})
	g.mu.Unlock(id)
	if err := g.Disconnect("foo"); err != nil {
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
