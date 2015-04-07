package api

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
	"github.com/NebulousLabs/Sia/types"
)

// addPeer creates a new serverTester and bootstraps it to st. It returns the
// new peer.
func (st *serverTester) addPeer(name string) *serverTester {
	// Mine a block on st, in the event that both st and newPeer are new, they
	// will be at the same height unless we mine a block on st.
	st.mineBlock()

	// Create a new peer and bootstrap it to st.
	newPeer := newServerTester(name, st.T)
	err := newPeer.gateway.Bootstrap(st.netAddress())
	if err != nil {
		st.Fatal("bootstrap failed:", err)
	}

	// Wait for bootstrapping to finish, then check that each has the same
	// number of peers and blocks.
	for len(st.gateway.Info().Peers) != len(newPeer.gateway.Info().Peers) {
		time.Sleep(time.Millisecond)
	}
	// force synchronization to st, in case newPeer tried to synchronize to
	// one of st's peers.
	for st.state.Height() != newPeer.state.Height() {
		newPeer.gateway.Synchronize(st.netAddress())
	}
	return newPeer
}

// TestPeering tests that peers are properly announced and relayed throughout
// the network.
func TestPeering(t *testing.T) {
	// Create to peers and add the first to the second.
	peer1 := newServerTester("TestPeering1", t)
	peer2 := newServerTester("TestPeering2", t)
	peer1.callAPI("/gateway/peer/add?address=" + string(peer2.netAddress()))

	// Check that the first has the second as a peer.
	var info modules.GatewayInfo
	peer1.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer2.netAddress() {
		t.Fatal("/gateway/peer/add did not add peer", peer2.netAddress())
	}

	// Create a third peer that bootstraps to the first peer and check that it
	// reports the others as peers.
	peer3 := peer1.addPeer("TestPeering3")
	peer3.getAPI("/gateway/status", &info)
	if len(info.Peers) != 2 {
		t.Fatal("bootstrap peer did not share its peers")
	}

	// peer2 should have received peer3 via peer1. Note that it does not have
	// peer1 though, because /gateway/add does not contact the added peer.
	peer2.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer3.netAddress() {
		t.Fatal("bootstrap peer did not relay the bootstrapping peer", info)
	}
}

// TestTransactionRelay checks that an unconfirmed transaction is relayed to
// all peers.
func TestTransactionRelay(t *testing.T) {
	// Create a server tester and give it a peer.
	st := newServerTester("TestTransactionRelay1", t)
	st2 := st.addPeer("TestTransactionRelay2")

	// Make sure both servers have empty transaction pools.
	tset := st.tpool.TransactionSet()
	tset2 := st2.tpool.TransactionSet()
	if len(tset) != 0 || len(tset2) != 0 {
		t.Fatal("transaction set is not empty after creating new server tester")
	}

	// Get the original balances of each server for later comparison.
	origBal := st.wallet.Balance(false)
	origBal2 := st2.wallet.Balance(false)

	// Create a transaction in the first server and check that it propagates to
	// the second. The check is done via spinning because network propagation
	// will take an unknown amount of time.
	st.callAPI("/wallet/send?amount=15&destination=" + st2.coinAddress())
	for len(tset) == 0 || len(tset2) == 0 {
		tset = st.tpool.TransactionSet()
		tset2 = st2.tpool.TransactionSet()
		time.Sleep(time.Millisecond)
	}

	// Check that the balances of each have updated appropriately, in
	// accordance with 0-conf.
	if origBal.Sub(types.NewCurrency64(15)).Cmp(st.wallet.Balance(false)) != 0 {
		t.Error(origBal.Big())
		t.Error(st.wallet.Balance(false).Big())
		t.Error("balances are incorrect for 0-conf transaction")
	}
	for origBal2.Add(types.NewCurrency64(15)).Cmp(st2.wallet.Balance(false)) != 0 {
		// t.Error(origBal2.Big())
		// t.Error(st2.wallet.Balance(false).Big())
		// t.Error("balances are incorrect for 0-conf transaction")
	}
}

// TestBlockBootstrap checks that gateway.Synchronize will be effective even
// when the first state has a few thousand blocks.
func TestBlockBootstrap(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Create a server and give it 2500 blocks.
	st := newServerTester("TestBlockBootstrap1", t)
	for i := 0; i < 2*gateway.MaxCatchUpBlocks+1; i++ {
		st.mineBlock()
	}

	// Add a peer and spin until the peer is caught up. addPeer() already does
	// this check, but it's left here to be explict anyway.
	st2 := st.addPeer("TestBlockBootstrap2")
	for st.state.Height() != st2.state.Height() {
		time.Sleep(time.Millisecond)
	}
}
