package gateway

import (
	"net"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
)

func TestRPCID(t *testing.T) {
	cases := map[rpcID]string{
		rpcID{}:                                       "        ",
		rpcID{'f', 'o', 'o'}:                          "foo     ",
		rpcID{'f', 'o', 'o', 'b', 'a', 'r', 'b', 'a'}: "foobarba",
	}
	for id, s := range cases {
		if id.String() != s {
			t.Error("rpcID.String mismatch: expected %v, got %v", s, id.String())
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
			t.Error("handlerName mismatch: expected %v, got %v", id, hid)
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
	defer g1.Close()

	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("failed to connect:", err)
	}

	g2.RegisterRPC("Foo", func(conn net.Conn) error {
		var i uint64
		err := encoding.ReadObject(conn, &i, 8)
		if err != nil {
			t.Error(err)
			return err
		} else if i == 0xdeadbeef {
			return encoding.WriteObject(conn, "foo")
		} else {
			return encoding.WriteObject(conn, "bar")
		}
	})

	var foo string
	err = g1.RPC(g2.Address(), "Foo", func(conn net.Conn) error {
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
	err = g1.RPC(g2.Address(), "Foo", func(conn net.Conn) error {
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

	// force a strike
	err = g1.RPC(g2.Address(), "Foo", func(net.Conn) error {
		return errNoPeers // any non-nil error will do
	})
	if err == nil {
		t.Fatal("bad RPC did not produce an error")
	}

	g1.peers[g2.Address()].sess.Close()
	if err := g1.RPC(g2.Address(), "Foo", nil); err == nil {
		t.Fatal("RPC on closed peer connection succeeded")
	}

	err = g1.Disconnect(g2.Address())
	if err != nil {
		t.Fatal("failed to disconnect:", err)
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

	g2.RegisterRPC("Foo", func(conn net.Conn) error {
		var i uint64
		err := encoding.ReadObject(conn, &i, 8)
		if err != nil {
			t.Error(err)
			return err
		} else if i == 0xdeadbeef {
			return encoding.WriteObject(conn, "foo")
		} else {
			return encoding.WriteObject(conn, "bar")
		}
	})

	// custom rpc fn (doesn't automatically write rpcID)
	rpcFn := func(fn func(net.Conn) error) error {
		conn, err := g1.peers[g2.Address()].sess.Open()
		if err != nil {
			return err
		}
		defer conn.Close()
		time.Sleep(10 * time.Millisecond)
		return fn(conn)
	}

	// bad rpcID
	err = rpcFn(func(conn net.Conn) error {
		return encoding.WriteObject(conn, [3]byte{1, 2, 3})
	})
	if err != nil {
		t.Fatal("rpcFn failed:", err)
	}

	// unknown rpcID
	err = rpcFn(func(conn net.Conn) error {
		return encoding.WriteObject(conn, handlerName("bar"))
	})
	if err != nil {
		t.Fatal("rpcFn failed:", err)
	}

	// valid rpcID
	err = rpcFn(func(conn net.Conn) error {
		return encoding.WriteObject(conn, handlerName("Foo"))
	})
	if err != nil {
		t.Fatal("rpcFn failed:", err)
	}
}

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
	doneChan := make(chan struct{})
	g2.RegisterRPC("Recv", func(conn net.Conn) error {
		encoding.ReadObject(conn, &g2Payload, 100)
		doneChan <- struct{}{}
		return nil
	})
	g3.RegisterRPC("Recv", func(conn net.Conn) error {
		encoding.ReadObject(conn, &g3Payload, 100)
		doneChan <- struct{}{}
		return nil
	})

	g1.Broadcast("Recv", "foo")
	<-doneChan
	<-doneChan
	if g2Payload != "foo" || g3Payload != "foo" {
		t.Fatal("broadcast failed:", g2Payload, g3Payload)
	}
}
