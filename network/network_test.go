package network

import (
	"net"
	"testing"
	"time"
)

var call1, call2 bool

type Foo struct{}

func (f Foo) Bar(int32) error { call1 = true; return nil }

func TestRegister(t *testing.T) {
	// create server
	tcps, err := NewTCPServer(9988)
	if err != nil {
		t.Fatal(err)
	}
	addr := NetAddress{"localhost", 9988}

	// register some handlers
	tcps.Register('F', func(net.Conn, []byte) error { call2 = true; return nil })
	tcps.Register('B', new(Foo).Bar)

	// call them
	err = addr.RPC('F', 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = addr.RPC('B', 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	// allow for message propagation
	time.Sleep(100 * time.Millisecond)

	// check that handlers were called
	if !call1 || !call2 {
		t.Fatal("Handler not called: call1 =", call1, "; call2 =", call2)
	}
}
