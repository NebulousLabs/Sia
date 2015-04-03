package gateway

import (
	"strconv"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/tester"
)

var (
	rpcPort = 10000
)

// newTestingGateway returns a gateway read to use in a testing environment.
func newTestingGateway(directory string, t *testing.T) *Gateway {
	gDir := tester.TempDir(directory, modules.GatewayDir)
	g, err := New(":"+strconv.Itoa(rpcPort), consensus.CreateGenesisState(), gDir)
	rpcPort++
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestTableTennis pings myAddr and checks the response.
func TestTableTennis(t *testing.T) {
	g := newTestingGateway("TestTableTennis", t)
	defer g.Close()
	if !g.Ping(g.myAddr) {
		t.Fatal("gateway did not respond to ping")
	}
}

func TestRPC(t *testing.T) {
	g := newTestingGateway("TestRPC", t)
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

// TestTimeout tests that connections time out properly.
func TestTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	g := newTestingGateway("TestTimeout - Good Peer", t)
	defer g.Close()

	// create unresponsive peer
	badpeer := newTestingGateway("TestTimeout - Bad Peer", t)
	// overwrite badpeer's Ping RPC with an incorrect one
	// since g is expecting 4 bytes, it will time out.
	badpeer.RegisterRPC("Ping", func(conn modules.NetConn) error {
		// write a length prefix, but no actual data
		conn.Write([]byte{1, 0, 0, 0, 0, 0, 0, 0})
		select {}
	})

	err := g.RPC(badpeer.Address(), "Ping", readerRPC([1]byte{}, 1))
	if err != ErrTimeout {
		t.Fatalf("Got wrong error: expected %v, got %v", ErrTimeout, err)
	}
}
