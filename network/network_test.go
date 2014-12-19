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
	tcps, err := NewTCPServer(":9987")
	if err != nil {
		t.Fatal(err)
	}

	// register some handlers
	tcps.Register("Foo", func(net.Conn) error { chan2 <- struct{}{}; return nil })
	tcps.Register("Bar", new(Foo).Bar)

	// call them
	err = tcps.myAddr.RPC("Foo", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = tcps.myAddr.RPC("Bar", 0, nil)
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

func TestPeerSharing(t *testing.T) {
	// create server
	tcps, err := NewTCPServer(":9981")
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
	tcps, err := NewTCPServer(":9005")
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
