package network

import (
	"net"
	"testing"
	"time"
)

var chan1 = make(chan struct{})
var chan2 = make(chan struct{})

type Foo struct{}

func (f Foo) Bar(int32) error { chan1 <- struct{}{}; return nil }

func TestRegister(t *testing.T) {
	// create server
	tcps, err := NewTCPServer(9988)
	if err != nil {
		t.Fatal(err)
	}
	addr := NetAddress{"localhost", 9988}

	// register some handlers
	tcps.Register("Foo", func(net.Conn, []byte) error { chan2 <- struct{}{}; return nil })
	tcps.Register("Bar", new(Foo).Bar)

	// call them
	err = addr.RPC("Foo", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = addr.RPC("Bar", 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		<-chan1
		<-chan2
		done <- struct{}{}
	}()

	// wait for messages to propagate
	select {
	// success
	case <-done:
		return

	// timeout
	case <-time.After(100 * time.Millisecond):
		t.Fatal("One or both handlers not called")
	}
}
