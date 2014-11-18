package siacore

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
	tcps.RegisterHandler('F', func(net.Conn, []byte) error { call2 = true; return nil })

	if err = tcps.RegisterRPC('B', new(Foo).Bar); err != nil {
		t.Fatal(err)
	}

	// call them
	err = addr.Call(func(conn net.Conn) error {
		_, err := conn.Write([]byte{'F', 0, 0, 0, 0})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = addr.Call(func(conn net.Conn) error {
		_, err := conn.Write([]byte{'B', 8, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0})
		return err
	})
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
