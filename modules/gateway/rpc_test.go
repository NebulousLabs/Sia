package gateway

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

func TestRPCID(t *testing.T) {
	cases := map[rpcID]string{
		rpcID{}:                                       "        ",
		rpcID{'f', 'o', 'o'}:                          "foo     ",
		rpcID{'f', 'o', 'o', 'b', 'a', 'r', 'b', 'a'}: "foobarba",
	}
	for id, s := range cases {
		if id.String() != s {
			t.Errorf("rpcID.String mismatch: expected %v, got %v", s, id.String())
		}
	}
}

func TestHandlerName(t *testing.T) {
	cases := map[string]rpcID{
		"":          rpcID{},
		"foo":       rpcID{'f', 'o', 'o'},
		"foobarbaz": rpcID{'f', 'o', 'o', 'b', 'a', 'r', 'b', 'a'},
	}
	for s, id := range cases {
		if hid := handlerName(s); hid != id {
			t.Errorf("handlerName mismatch: expected %v, got %v", id, hid)
		}
	}
}

func TestRPC(t *testing.T) {
	g1 := newTestingGateway("TestRPC1", t)
	defer g1.Close()

	if err := g1.RPC("foo", "", nil); err == nil {
		t.Fatal("RPC on unconnected peer succeded")
	}

	g2 := newTestingGateway("TestRPC2", t)
	defer g2.Close()

	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("failed to connect:", err)
	}

	g2.RegisterRPC("Foo", func(conn modules.PeerConn) error {
		var i uint64
		err := encoding.ReadObject(conn, &i, 8)
		if err != nil {
			return err
		} else if i == 0xdeadbeef {
			return encoding.WriteObject(conn, "foo")
		} else {
			return encoding.WriteObject(conn, "bar")
		}
	})

	var foo string
	err = g1.RPC(g2.Address(), "Foo", func(conn modules.PeerConn) error {
		err := encoding.WriteObject(conn, 0xdeadbeef)
		if err != nil {
			return err
		}
		return encoding.ReadObject(conn, &foo, 11)
	})
	if err != nil {
		t.Fatal(err)
	}
	if foo != "foo" {
		t.Fatal("Foo gave wrong response:", foo)
	}

	// wrong number should produce an error
	err = g1.RPC(g2.Address(), "Foo", func(conn modules.PeerConn) error {
		err := encoding.WriteObject(conn, 0xbadbeef)
		if err != nil {
			return err
		}
		return encoding.ReadObject(conn, &foo, 11)
	})
	if err != nil {
		t.Fatal(err)
	}
	if foo != "bar" {
		t.Fatal("Foo gave wrong response:", foo)
	}

	// don't read or write anything
	err = g1.RPC(g2.Address(), "Foo", func(modules.PeerConn) error {
		return errNoPeers // any non-nil error will do
	})
	if err == nil {
		t.Fatal("bad RPC did not produce an error")
	}

	g1.peers[g2.Address()].sess.Close()
	if err := g1.RPC(g2.Address(), "Foo", nil); err == nil {
		t.Fatal("RPC on closed peer connection succeeded")
	}
}

func TestThreadedHandleConn(t *testing.T) {
	g1 := newTestingGateway("TestThreadedHandleConn1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestThreadedHandleConn2", t)
	defer g2.Close()

	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("failed to connect:", err)
	}

	g2.RegisterRPC("Foo", func(conn modules.PeerConn) error {
		var i uint64
		err := encoding.ReadObject(conn, &i, 8)
		if err != nil {
			return err
		} else if i == 0xdeadbeef {
			return encoding.WriteObject(conn, "foo")
		} else {
			return encoding.WriteObject(conn, "bar")
		}
	})

	// custom rpc fn (doesn't automatically write rpcID)
	rpcFn := func(fn func(modules.PeerConn) error) error {
		conn, err := g1.peers[g2.Address()].open()
		if err != nil {
			return err
		}
		defer conn.Close()
		return fn(conn)
	}

	// bad rpcID
	err = rpcFn(func(conn modules.PeerConn) error {
		return encoding.WriteObject(conn, [3]byte{1, 2, 3})
	})
	if err != nil {
		t.Fatal("rpcFn failed:", err)
	}

	// unknown rpcID
	err = rpcFn(func(conn modules.PeerConn) error {
		return encoding.WriteObject(conn, handlerName("bar"))
	})
	if err != nil {
		t.Fatal("rpcFn failed:", err)
	}

	// valid rpcID
	err = rpcFn(func(conn modules.PeerConn) error {
		return encoding.WriteObject(conn, handlerName("Foo"))
	})
	if err != nil {
		t.Fatal("rpcFn failed:", err)
	}
}

// TestBroadcast tests that calling broadcast with a slice of peers only
// broadcasts to those peers.
func TestBroadcast(t *testing.T) {
	g1 := newTestingGateway("TestBroadcast1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestBroadcast2", t)
	defer g2.Close()
	g3 := newTestingGateway("TestBroadcast3", t)
	defer g3.Close()

	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("failed to connect:", err)
	}
	err = g1.Connect(g3.Address())
	if err != nil {
		t.Fatal("failed to connect:", err)
	}

	var g2Payload, g3Payload string
	g2DoneChan := make(chan struct{})
	g3DoneChan := make(chan struct{})
	bothDoneChan := make(chan struct{})

	g2.RegisterRPC("Recv", func(conn modules.PeerConn) error {
		encoding.ReadObject(conn, &g2Payload, 100)
		g2DoneChan <- struct{}{}
		return nil
	})
	g3.RegisterRPC("Recv", func(conn modules.PeerConn) error {
		encoding.ReadObject(conn, &g3Payload, 100)
		g3DoneChan <- struct{}{}
		return nil
	})

	// Test that broadcasting to all peers in g1.Peers() broadcasts to all peers.
	peers := g1.Peers()
	g1.Broadcast("Recv", "bar", peers)
	go func() {
		<-g2DoneChan
		<-g3DoneChan
		bothDoneChan <- struct{}{}
	}()
	select {
	case <-bothDoneChan:
		// Both g2 and g3 should receive the broadcast.
	case <-time.After(10 * time.Millisecond):
		t.Fatal("broadcasting to gateway.Peers() should broadcast to all peers")
	}
	if g2Payload != "bar" || g3Payload != "bar" {
		t.Fatal("broadcast failed:", g2Payload, g3Payload)
	}

	// Test that broadcasting to only g2 does not broadcast to g3.
	peers = make([]modules.Peer, 0)
	for _, p := range g1.Peers() {
		if p.NetAddress == g2.Address() {
			peers = append(peers, p)
			break
		}
	}
	g1.Broadcast("Recv", "baz", peers)
	select {
	case <-g2DoneChan:
		// Only g2 should receive a broadcast.
	case <-g3DoneChan:
		t.Error("broadcast broadcasted to peers not in the peers arg")
	case <-time.After(10 * time.Millisecond):
		t.Fatal("called broadcast with g2 in peers list, but g2 didn't receive it.")
	}
	if g2Payload != "baz" {
		t.Fatal("broadcast failed:", g2Payload)
	}

	// Test that broadcasting to only g3 does not broadcast to g2.
	peers = make([]modules.Peer, 0)
	for _, p := range g1.Peers() {
		if p.NetAddress == g3.Address() {
			peers = append(peers, p)
			break
		}
	}
	g1.Broadcast("Recv", "qux", peers)
	select {
	case <-g2DoneChan:
		t.Error("broadcast broadcasted to peers not in the peers arg")
	case <-g3DoneChan:
		// Only g3 should receive a broadcast.
	case <-time.After(10 * time.Millisecond):
		t.Fatal("called broadcast with g3 in peers list, but g3 didn't receive it.")
	}
	if g3Payload != "qux" {
		t.Fatal("broadcast failed:", g3Payload)
	}

	// Test that broadcasting to an empty slice (but not nil!) does not broadcast
	// to g2 or g3.
	peers = make([]modules.Peer, 0)
	g1.Broadcast("Recv", "quux", peers)
	select {
	case <-g2DoneChan:
		t.Error("broadcast broadcasted to peers not in the peers arg")
	case <-g3DoneChan:
		t.Error("broadcast broadcasted to peers not in the peers arg")
	case <-time.After(10 * time.Millisecond):
		// Neither peer should receive a broadcast.
	}

	// Test that calling broadcast with nil peers does not broadcast to g2 or g3.
	g1.Broadcast("Recv", "foo", nil)
	select {
	case <-g2DoneChan:
		t.Error("broadcast broadcasted to peers not in the peers arg")
	case <-g3DoneChan:
		t.Error("broadcast broadcasted to peers not in the peers arg")
	case <-time.After(10 * time.Millisecond):
		// Neither peer should receive a broadcast.
	}
}
