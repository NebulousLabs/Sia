package api

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/modules/tester"
	"github.com/NebulousLabs/Sia/types"
)

// addPeer creates a new serverTester and bootstraps it to st. It returns the
// new peer.
func (st *serverTester) addPeer(name string) *serverTester {
	_, _, err := st.miner.FindBlock()
	if err != nil {
		st.t.Fatal(err)
	}
	st.csUpdateWait()

	// Create a new peer and bootstrap it to st.
	newPeer := newServerTester(name, st.t)
	err = newPeer.server.gateway.Connect(st.netAddress())
	if err != nil {
		st.t.Fatal("bootstrap failed:", err)
	}

	// Synchronize the consensus sets of newPeer and st.
	err = newPeer.server.cs.Synchronize(st.netAddress())
	if err != nil {
		st.t.Fatal("synchronize failed:", err)
	}

	return newPeer
}

func TestGatewayStatus(t *testing.T) {
	st := newServerTester("TestGatewayStatus", t)
	var info GatewayInfo
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 0 {
		t.Fatal("/gateway/status gave bad peer list:", info.Peers)
	}
}

func TestGatewayPeerAdd(t *testing.T) {
	st := newServerTester("TestGatewayPeerAdd", t)
	peer, err := gateway.New(":0", tester.TempDir("api", "TestGatewayPeerAdd", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	st.callAPI("/gateway/peer/add?address=" + string(peer.Address()))

	var info GatewayInfo
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer.Address() {
		t.Fatal("/gateway/peer/add did not add peer", peer.Address())
	}
}

func TestGatewayPeerRemove(t *testing.T) {
	st := newServerTester("TestGatewayPeerRemove", t)
	peer, err := gateway.New(":0", tester.TempDir("api", "TestGatewayPeerRemove", "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	st.callAPI("/gateway/peer/add?address=" + string(peer.Address()))

	var info GatewayInfo
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer.Address() {
		t.Fatal("/gateway/peer/add did not add peer", peer.Address())
	}

	st.callAPI("/gateway/peer/remove?address=" + string(peer.Address()))
	st.getAPI("/gateway/status", &info)
	if len(info.Peers) != 0 {
		t.Fatal("/gateway/peer/add did not add peer", peer.Address())
	}
}

// TestTransactionRelay checks that an unconfirmed transaction is relayed to
// all peers.
func TestTransactionRelay(t *testing.T) {
	t.Skip("TODO: Broken")

	// Create a server tester and give it a peer.
	st := newServerTester("TestTransactionRelay1", t)
	st2 := st.addPeer("TestTransactionRelay2")

	// Make sure both servers have empty transaction pools.
	tset := st.server.tpool.TransactionSet()
	tset2 := st2.server.tpool.TransactionSet()
	if len(tset) != 0 || len(tset2) != 0 {
		t.Fatal("transaction set is not empty after creating new server tester")
	}

	// Get the original balances of each server for later comparison.
	origBal := st.server.wallet.Balance(false)
	origBal2 := st2.server.wallet.Balance(false)

	// Create a transaction in the first server and check that it propagates to
	// the second. The check is done via spinning because network propagation
	// will take an unknown amount of time.
	st.callAPI("/wallet/send?amount=15&destination=" + st2.coinAddress())
	for len(tset) == 0 || len(tset2) == 0 {
		tset = st.server.tpool.TransactionSet()
		tset2 = st2.server.tpool.TransactionSet()
		time.Sleep(time.Millisecond)
	}

	// Check that the balances of each have updated appropriately, in
	// accordance with 0-conf.
	if origBal.Sub(types.NewCurrency64(15)).Cmp(st.server.wallet.Balance(false)) != 0 {
		t.Error(origBal.Big())
		t.Error(st.server.wallet.Balance(false).Big())
		t.Error("balances are incorrect for 0-conf transaction")
	}
	for origBal2.Add(types.NewCurrency64(15)).Cmp(st2.server.wallet.Balance(false)) != 0 {
		// t.Error(origBal2.Big())
		// t.Error(st2.wallet.Balance(false).Big())
		// t.Error("balances are incorrect for 0-conf transaction")
	}
}
