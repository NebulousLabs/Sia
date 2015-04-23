package gateway

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/modules/tester"
)

// newTestingGateway returns a gateway read to use in a testing environment.
func newTestingGateway(name string, t *testing.T) *Gateway {
	testdir := tester.TempDir("gateway", name)
	s, err := consensus.New(filepath.Join(testdir, modules.ConsensusDir))
	if err != nil {
		t.Fatal(err)
	}
	g, err := New(":0", s, testdir)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestRPC(t *testing.T) {
	g1 := newTestingGateway("TestRPC1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestRPC2", t)
	defer g1.Close()

	g1.RegisterRPC("Foo", func(conn net.Conn) error {
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

	err := g2.Connect(g1.Address())
	if err != nil {
		t.Fatal("failed to connect:", err)
	}

	var foo string
	err = g2.RPC(g1.Address(), "Foo", func(conn net.Conn) error {
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
	err = g2.RPC(g1.Address(), "Foo", func(conn net.Conn) error {
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
}

/*

// TestTimeout tests that connections time out properly.
// TODO: bring back connection monitoring
func TestTimeout(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	g := newTestingGateway("TestTimeout", t)
	defer g.Close()

	// create unresponsive peer
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal("listen failed:", err)
	}
	go func() {
		l.Accept()
		select {}
	}()

	_, err = g.Connect(modules.NetAddress(l.Addr().String()))
	ne, ok := err.(net.Error)
	if err == nil || !ok || !ne.Timeout() {
		t.Fatalf("Got wrong error: expected timeout, got %v", err)
	}
}

*/
