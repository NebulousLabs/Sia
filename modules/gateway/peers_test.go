package gateway

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/inconshreveable/muxado"
)

func TestAddPeer(t *testing.T) {
	g := newTestingGateway("TestAddPeer", t)
	defer g.Close()
	g.addPeer(&peer{addr: "foo", sess: muxado.Client(nil)})
	if len(g.peers) != 1 {
		t.Fatal("gateway did not add peer")
	}
	if len(g.nodes) != 2 {
		t.Fatal("gateway did not add node")
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
	if err := encoding.WriteObject(conn, "foo"); err != nil {
		t.Fatal("couldn't write address")
	}
	// g should add foo
	for g.peers["foo"] == nil {
	}
	conn.Close()
	// g should remove foo
	for g.peers["foo"] != nil {
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
	g := newTestingGateway("TestConnect", t)
	defer g.Close()

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

	if err := g.Connect(modules.NetAddress(l.Addr().String())); err != nil {
		t.Fatal("connect failed:", err)
	}

	if len(g.peers) != 1 {
		t.Fatal("gateway did not add peer after connecting:", g.peers)
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

	conn, err := net.Dial("tcp", string(g.Address()))
	if err != nil {
		t.Fatal("dial failed:", err)
	}
	g.addPeer(&peer{addr: "foo", sess: muxado.Client(conn)})
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
	for i := 0; i < 8; i++ {
		peerAddr := modules.NetAddress("foo" + strconv.Itoa(i))
		g1.addPeer(&peer{addr: peerAddr, sess: muxado.Client(nil)})
	}

	// makeOutboundConnections should now sleep for 5 seconds
	time.Sleep(1 * time.Second)
	// remove a peer while makeOutboundConnections is asleep, and add a new
	// connectable address to the node list
	g1.Disconnect("foo1")
	g2 := newTestingGateway("TestMakeOutboundConnections2", t)
	defer g2.Close()
	g1.addNode(g2.Address())

	// when makeOutboundConnections wakes up, it should connect to g2.
	time.Sleep(5 * time.Second)
	if len(g1.peers) != 8 {
		t.Fatal("gateway did not reach 8 peers:", g1.peers)
	}
	if g1.peers[g2.Address()] == nil {
		t.Fatal("gateway did not connect to g2")
	}
}
