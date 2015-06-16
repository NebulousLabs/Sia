package gateway

import (
	"net"
	"strconv"
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

func TestListen(t *testing.T) {
	g := newTestingGateway("TestListen", t)
	defer g.Close()

	// "compliant" connect
	conn, err := net.Dial("tcp", string(g.Address()))
	if err != nil {
		t.Fatal("dial failed:", err)
	}
	addr := modules.NetAddress(conn.LocalAddr().String())
	// send version
	if err := encoding.WriteObject(conn, build.Version); err != nil {
		t.Fatal("couldn't write version")
	}
	// read ack
	var ack string
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

	// a simple 'conn.Close' would not obey the muxado disconnect protocol
	muxado.Client(conn).Close()

	// g should remove the peer
	for ok {
		id := g.mu.RLock()
		_, ok = g.peers[addr]
		g.mu.RUnlock(id)
	}

	// "uncompliant" connect
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
	id = g.mu.RLock()
	defer g.mu.RUnlock(id)
	if _, ok := g.nodes[dummyNode]; !ok {
		t.Fatal("bootstrapper should have received dummyNode:", g.nodes)
	}
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
		conn, err := l.Accept()
		if err != nil {
			t.Fatal("accept failed:", err)
		}
		conn.Close()
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

func TestMakeOutboundConnections(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g1 := newTestingGateway("TestMakeOutboundConnections1", t)
	defer g1.Close()

	// first add 8 dummy peers
	id := g1.mu.Lock()
	for i := 0; i < 8; i++ {
		peerAddr := modules.NetAddress("foo" + strconv.Itoa(i))
		g1.peers[peerAddr] = &peer{addr: peerAddr, sess: nil}
	}
	g1.mu.Unlock(id)

	// makeOutboundConnections should now sleep for 5 seconds
	time.Sleep(1 * time.Second)

	// remove a peer while makeOutboundConnections is asleep, and add a new
	// connectable address to the node list
	id = g1.mu.Lock()
	delete(g1.peers, "foo1")
	g1.mu.Unlock(id)

	g2 := newTestingGateway("TestMakeOutboundConnections2", t)
	defer g2.Close()
	id = g1.mu.Lock()
	g1.nodes[g2.Address()] = struct{}{} // manual insertion to bypass addNode
	g1.mu.Unlock(id)

	// when makeOutboundConnections wakes up, it should connect to g2.
	time.Sleep(5 * time.Second)

	id = g1.mu.RLock()
	defer g1.mu.RUnlock(id)
	if len(g1.peers) != 8 {
		t.Fatal("gateway did not reach 8 peers:", g1.peers)
	}
	if g1.peers[g2.Address()] == nil {
		t.Fatal("gateway did not connect to g2")
	}
}
