package gateway

import (
	"net"
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
)

func TestRPC(t *testing.T) {
	g1 := newTestingGateway("TestRPC1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestRPC2", t)
	defer g1.Close()

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

	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("failed to connect:", err)
	}

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

	err = g1.Disconnect(g2.Address())
	if err != nil {
		t.Fatal("failed to disconnect:", err)
	}
}
