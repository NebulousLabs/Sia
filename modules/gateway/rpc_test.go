package gateway

import (
	"io"
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
		"":          {},
		"foo":       {'f', 'o', 'o'},
		"foobarbaz": {'f', 'o', 'o', 'b', 'a', 'r', 'b', 'a'},
	}
	for s, id := range cases {
		if hid := handlerName(s); hid != id {
			t.Errorf("handlerName mismatch: expected %v, got %v", id, hid)
		}
	}
}

// TestRegisterRPC tests that registering the same RPC twice causes a panic.
func TestRegisterRPC(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	g := newTestingGateway("TestRegisterRPC", t)
	defer g.Close()

	g.RegisterRPC("Foo", func(conn modules.PeerConn) error { return nil })
	defer func() {
		if r := recover(); r == nil {
			t.Error("Registering the same RPC twice did not cause a panic")
		}
	}()
	g.RegisterRPC("Foo", func(conn modules.PeerConn) error { return nil })
}

// TestUnregisterRPC tests that unregistering an RPC causes calls to it to
// fail, and checks that unregistering a non-registered RPC causes a panic.
func TestUnregisterRPC(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	g1 := newTestingGateway("TestUnregisterRPC1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestUnregisterRPC2", t)
	defer g2.Close()

	err := g2.Connect(g1.Address())
	if err != nil {
		t.Fatal(err)
	}

	dummyFunc := func(conn modules.PeerConn) error {
		var str string
		return encoding.ReadObject(conn, &str, 11)
	}

	// Register RPC and check that calling it succeeds.
	g1.RegisterRPC("Foo", func(conn modules.PeerConn) error {
		return encoding.WriteObject(conn, "foo")
	})
	err = g2.RPC(g1.Address(), "Foo", dummyFunc)
	if err != nil {
		t.Errorf("calling registered RPC on g1 returned %q", err)
	}
	// Unregister RPC and check that calling it fails.
	g1.UnregisterRPC("Foo")
	err = g2.RPC(g1.Address(), "Foo", dummyFunc)
	if err != io.EOF {
		t.Errorf("calling unregistered RPC on g1 returned %q instead of io.EOF", err)
	}

	// Unregister again and check that it panics.
	defer func() {
		if r := recover(); r == nil {
			t.Error("Unregistering an unregistered RPC did not cause a panic")
		}
	}()
	g1.UnregisterRPC("Foo")
}

// TestRegisterConnectCall tests that registering the same on-connect call
// twice causes a panic.
func TestRegisterConnectCall(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	g := newTestingGateway("TestRegisterConnectCall", t)
	defer g.Close()

	// Register an on-connect call.
	g.RegisterConnectCall("Foo", func(conn modules.PeerConn) error { return nil })
	defer func() {
		if r := recover(); r == nil {
			t.Error("Registering the same on-connect call twice did not cause a panic")
		}
	}()
	g.RegisterConnectCall("Foo", func(conn modules.PeerConn) error { return nil })
}

// TestUnregisterConnectCallPanics tests that unregistering the same on-connect
// call twice causes a panic.
func TestUnregisterConnectCallPanics(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	g1 := newTestingGateway("TestUnregisterConnectCall1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestUnregisterConnectCall2", t)
	defer g2.Close()

	rpcChan := make(chan struct{})

	// Register on-connect call and test that RPC is called on connect.
	g1.RegisterConnectCall("Foo", func(conn modules.PeerConn) error {
		rpcChan <- struct{}{}
		return nil
	})
	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-rpcChan:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("ConnectCall not called on Connect after it was registered")
	}
	// Disconnect, unregister on-connect call, and test that RPC is not called on connect.
	err = g1.Disconnect(g2.Address())
	if err != nil {
		t.Fatal(err)
	}
	g1.UnregisterConnectCall("Foo")
	err = g1.Connect(g2.Address())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-rpcChan:
		t.Fatal("ConnectCall called on Connect after it was unregistered")
	case <-time.After(200 * time.Millisecond):
	}
	// Unregister again and check that it panics.
	defer func() {
		if r := recover(); r == nil {
			t.Error("Unregistering an unregistered on-connect call did not cause a panic")
		}
	}()
	g1.UnregisterConnectCall("Foo")
}

func TestRPC(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g1 := newTestingGateway("TestRPC1", t)
	defer g1.Close()

	if err := g1.RPC("foo.com:123", "", nil); err == nil {
		t.Fatal("RPC on unconnected peer succeeded")
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
	if testing.Short() {
		t.SkipNow()
	}

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
	if testing.Short() {
		t.SkipNow()
	}

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
	case <-time.After(200 * time.Millisecond):
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
	case <-time.After(200 * time.Millisecond):
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
	case <-time.After(200 * time.Millisecond):
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
	case <-time.After(200 * time.Millisecond):
		// Neither peer should receive a broadcast.
	}

	// Test that calling broadcast with nil peers does not broadcast to g2 or g3.
	g1.Broadcast("Recv", "foo", nil)
	select {
	case <-g2DoneChan:
		t.Error("broadcast broadcasted to peers not in the peers arg")
	case <-g3DoneChan:
		t.Error("broadcast broadcasted to peers not in the peers arg")
	case <-time.After(200 * time.Millisecond):
		// Neither peer should receive a broadcast.
	}
}

// TestOutboundAndInboundRPCs tests that both inbound and outbound connections
// can successfully make RPC calls.
func TestOutboundAndInboundRPCs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g1 := newTestingGateway("TestRPC1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestRPC2", t)
	defer g2.Close()

	rpcChanG1 := make(chan struct{})
	rpcChanG2 := make(chan struct{})

	g1.RegisterRPC("recv", func(conn modules.PeerConn) error {
		rpcChanG1 <- struct{}{}
		return nil
	})
	g2.RegisterRPC("recv", func(conn modules.PeerConn) error {
		rpcChanG2 <- struct{}{}
		return nil
	})

	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	err = g1.RPC(g2.Address(), "recv", func(conn modules.PeerConn) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	<-rpcChanG2

	// Call the "recv" RPC on g1. We don't know g1's address as g2 sees it, so we
	// get it from the first address in g2's peer list.
	var addr modules.NetAddress
	for p_addr := range g2.peers {
		addr = p_addr
		break
	}
	err = g2.RPC(addr, "recv", func(conn modules.PeerConn) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	<-rpcChanG1
}

// TestCallingRPCFromRPC tests that calling an RPC from an RPC works.
func TestCallingRPCFromRPC(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g1 := newTestingGateway("TestCallingRPCFromRPC1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestCallingRPCFromRPC2", t)
	defer g2.Close()

	errChan := make(chan error)
	g1.RegisterRPC("FOO", func(conn modules.PeerConn) error {
		err := g1.RPC(conn.RPCAddr(), "BAR", func(conn modules.PeerConn) error { return nil })
		errChan <- err
		return err
	})

	barChan := make(chan struct{})
	g2.RegisterRPC("BAR", func(conn modules.PeerConn) error {
		barChan <- struct{}{}
		return nil
	})

	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal(err)
	}

	// Call the "FOO" RPC on g1. We don't know g1's address as g2 sees it, so we
	// get it from the first address in g2's peer list.
	var addr modules.NetAddress
	for _, p := range g2.Peers() {
		addr = p.NetAddress
		break
	}
	err = g2.RPC(addr, "FOO", func(conn modules.PeerConn) error { return nil })

	select {
	case err = <-errChan:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected FOO RPC to be called")
	}

	select {
	case <-barChan:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected BAR RPC to be called")
	}
}
