package network

import (
	"testing"
)

type Foo struct{}

func (f Foo) Bar(i uint32) (s string, err error) {
	if i == 0xdeadbeef {
		s = "bar"
	}
	return
}

func TestRegister(t *testing.T) {
	// create server
	tcps, err := NewTCPServer(":9000")
	if err != nil {
		t.Fatal(err)
	}

	// register some handlers
	tcps.Register("Foo", func() (string, error) { return "foo", nil })
	tcps.Register("Bar", new(Foo).Bar)

	// call them
	var foo string
	err = tcps.myAddr.RPC("Foo", nil, &foo)
	if err != nil {
		t.Fatal(err)
	}
	if foo != "foo" {
		t.Fatalf("Foo was not called")
	}

	var bar string
	err = tcps.myAddr.RPC("Bar", 0xdeadbeef, &bar)
	if err != nil {
		t.Fatal(err)
	}
	if bar != "bar" {
		t.Fatalf("Bar was not called")
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

func TestPeerSharing(t *testing.T) {
	// create server
	tcps, err := NewTCPServer(":9002")
	if err != nil {
		t.Fatal(err)
	}

	// add a peer
	peer := Address("foo:9001")
	tcps.AddPeer(peer)
	// tcps only has one peer, so RandomPeer() should return peer
	if tcps.RandomPeer() != peer {
		t.Fatal("server has bad peer list:", tcps.AddressBook())
	}

	// ask tcps for peers
	var resp []Address
	err = tcps.myAddr.RPC("SharePeers", nil, &resp)
	if err != nil {
		t.Fatal(err)
	}
	// resp should be exactly []Address{peer}
	if len(resp) != 1 || resp[0] != peer {
		t.Fatal("server gave bad peer list:", resp)
	}

	// add a couple more peers
	tcps.AddPeer(Address("bar:9002"))
	tcps.AddPeer(Address("baz:9003"))
	tcps.AddPeer(Address("quux:9004"))
	err = tcps.myAddr.RPC("SharePeers", nil, &resp)
	if err != nil {
		t.Fatal(err)
	}
	// resp should now contain 4 distinct addresses
	for i := 0; i < len(resp); i++ {
		for j := i + 1; j < len(resp); j++ {
			if resp[i] == resp[j] {
				t.Fatal("resp contains duplicate addresses:", resp)
			}
		}
	}
}

func TestPeerCulling(t *testing.T) {
	// this test necessitates a timeout
	if testing.Short() {
		t.Skip()
	}

	// create server
	tcps, err := NewTCPServer(":9003")
	if err != nil {
		t.Fatal(err)
	}

	// add google as a peer
	peer := Address("8.8.8.8:9001")
	tcps.AddPeer(peer)

	// send a broadcast
	// doesn't need to be a real RPC
	tcps.Broadcast("QuestionWhoseAnswerIs", 42, nil)

	// peer should have been removed
	if len(tcps.AddressBook()) != 0 {
		t.Fatal("server did not remove dead peer:", tcps.AddressBook())
	}
}
