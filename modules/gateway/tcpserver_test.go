package gateway

import (
	"errors"
	"testing"
)

type Foo struct{}

func (f Foo) Bar(i uint32) (s string, err error) {
	if i == 0xdeadbeef {
		s = "bar"
	} else {
		err = errors.New("wrong number")
	}
	return
}

func TestRegisterRPC(t *testing.T) {
	// create server
	tcps, err := NewTCPServer(":9000")
	if err != nil {
		t.Fatal(err)
	}

	// register some handlers
	err = tcps.RegisterRPC("Foo", func() (string, error) { return "foo", nil })
	if err != nil {
		t.Fatal(err)
	}
	err = tcps.RegisterRPC("Bar", new(Foo).Bar)
	if err != nil {
		t.Fatal(err)
	}

	// call them
	var foo string
	err = tcps.myAddr.RPC("Foo", nil, &foo)
	if err != nil {
		t.Fatal(err)
	}
	if foo != "foo" {
		t.Fatal("Foo was not called")
	}

	var bar string
	err = tcps.myAddr.RPC("Bar", 0xdeadbeef, &bar)
	if err != nil {
		t.Fatal(err)
	}
	if bar != "bar" {
		t.Fatal("Bar was not called")
	}

	// wrong number should produce an error
	err = tcps.myAddr.RPC("Bar", 0xbadbeef, &bar)
	if err == nil || err.Error() != "wrong number" {
		t.Fatal("Bar returned nil or incorrect error:", err)
	}
}

func TestTableTennis(t *testing.T) {
	// create server
	tcps, err := NewTCPServer(":9001")
	if err != nil {
		t.Fatal(err)
	}
	if !Ping(tcps.myAddr) {
		t.Fatal("server did not respond to ping")
	}
}
