package gateway

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

func TestCallbackAddr(t *testing.T) {
	g1 := newTestingGateway("TestCallbackAddr1", t)
	defer g1.Close()
	g2 := newTestingGateway("TestCallbackAddr2", t)
	defer g2.Close()

	err := g1.Connect(g2.Address())
	if err != nil {
		t.Fatal("failed to connect:", err)
	}

	var g1addr, g2addr modules.NetAddress
	g2.RegisterRPC("Foo", func(conn modules.PeerConn) error {
		g1addr = conn.CallbackAddr()
		return nil
	})
	err = g1.RPC(g2.Address(), "Foo", func(conn modules.PeerConn) error {
		g2addr = conn.CallbackAddr()
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g1addr != g1.Address() {
		t.Errorf("CallbackAddr returned %v, expected %v", g1addr, g1.Address())
	} else if g2addr != g2.Address() {
		t.Errorf("CallbackAddr returned %v, expected %v", g2addr, g2.Address())
	}
}
