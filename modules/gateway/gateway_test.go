package gateway

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

var port = 9001

func newTestGateway(t *testing.T) *Gateway {
	g, err := New(":"+strconv.Itoa(port), consensus.CreateGenesisState(), "")
	if err != nil {
		t.Fatal(err)
	}
	port++
	return g
}

func TestTableTennis(t *testing.T) {
	g := newTestGateway(t)
	defer g.Close()
	if !g.Ping(g.myAddr) {
		t.Fatal("gateway did not respond to ping")
	}
}

func TestRPC(t *testing.T) {
	g := newTestGateway(t)
	defer g.Close()

	g.RegisterRPC("Foo", func(conn modules.NetConn) error {
		var i uint64
		err := conn.ReadObject(&i, 8)
		if err != nil {
			t.Error(err)
			return err
		} else if i == 0xdeadbeef {
			return conn.WriteObject("foo")
		} else {
			return conn.WriteObject("bar")
		}
	})

	var foo string
	err := g.RPC(g.myAddr, "Foo", func(conn modules.NetConn) error {
		err := conn.WriteObject(0xdeadbeef)
		if err != nil {
			return err
		}
		return conn.ReadObject(&foo, 11)
	})
	if err != nil {
		t.Fatal(err)
	}
	if foo != "foo" {
		t.Fatal("Foo gave wrong response:", foo)
	}

	// wrong number should produce an error
	err = g.RPC(g.myAddr, "Foo", func(conn modules.NetConn) error {
		err := conn.WriteObject(0xbadbeef)
		if err != nil {
			return err
		}
		return conn.ReadObject(&foo, 11)
	})
	if err != nil {
		t.Fatal(err)
	}
	if foo != "bar" {
		t.Fatal("Foo gave wrong response:", foo)
	}
}
