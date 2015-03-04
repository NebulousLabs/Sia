package main

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
)

// addPeer creates a new daemonTester and bootstraps it to dt. It returns the
// new peer.
func (dt *daemonTester) addPeer() *daemonTester {
	// Mine a block on dt, in the event that both dt and newPeer are new, they
	// will be at the same height unless we mine a block on dt.
	dt.mineBlock()

	// Create a new peer and bootstrap it to dt.
	newPeer := newDaemonTester(dt.T)
	err := newPeer.gateway.Bootstrap(dt.netAddress())
	if err != nil {
		dt.Fatal("bootstrap failed:", err)
	}

	// Wait for RPC to finish, then check that each has the same number of
	// peers and blocks.
	for len(dt.gateway.Info().Peers) != len(newPeer.gateway.Info().Peers) {
		time.Sleep(time.Millisecond)
	}
	for dt.state.Height() != newPeer.state.Height() {
		time.Sleep(time.Millisecond)
	}
	return newPeer
}

// TestPeering tests that peers are properly announced and relayed throughout
// the network.
func TestPeering(t *testing.T) {
	// Create to peers and add the first to the second.
	peer1 := newDaemonTester(t)
	peer2 := newDaemonTester(t)
	peer1.callAPI("/gateway/add?addr=" + string(peer2.netAddress()))

	// Check that the first has the second as a peer.
	var info modules.GatewayInfo
	peer1.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer2.netAddress() {
		t.Fatal("/gateway/add did not add peer", peer2.netAddress())
	}

	// Create a third peer that bootstraps to the first peer and check that it
	// reports the others as peers.
	peer3 := peer1.addPeer()
	peer3.getAPI("/gateway/status", &info)
	if len(info.Peers) != 2 {
		t.Fatal("bootstrap peer did not share its peers")
	}

	// peer2 should have received peer3 via peer1. Note that it does not have
	// peer1 though, because /gateway/add does not contact the added peer.
	peer2.getAPI("/gateway/status", &info)
	if len(info.Peers) != 1 || info.Peers[0] != peer3.netAddress() {
		t.Fatal("bootstrap peer did not relay the bootstrapping peer")
	}
}

// TestTransactionRelay checks that an unconfirmed transaction is relayed to
// all peers.
func TestTransactionRelay(t *testing.T) {
	// Create a daemon tester and give it a peer.
	dt := newDaemonTester(t)
	dt2 := dt.addPeer()

	// Make sure both daemons have empty transaction pools.
	tset, err := dt.tpool.TransactionSet()
	if err != nil {
		t.Fatal(err)
	}
	tset2, err := dt2.tpool.TransactionSet()
	if err != nil {
		t.Fatal(err)
	}
	if len(tset) != 0 || len(tset2) != 0 {
		t.Fatal("transaction set is not empty after creating new daemon tester")
	}

	// Get the original balances of each daemon for later comparison.
	origBal := dt.wallet.Balance(false)
	origBal2 := dt2.wallet.Balance(false)

	// Create a transaction in the first daemon and check that it propagates to
	// the second. The check is done via spinning because network propagation
	// will take an unknown amount of time.
	dt.callAPI("/wallet/send?amount=15&dest=" + dt2.coinAddress())
	for len(tset) == 0 || len(tset2) == 0 {
		tset, err = dt.tpool.TransactionSet()
		if err != nil {
			t.Fatal(err)
		}
		tset2, err = dt2.tpool.TransactionSet()
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Millisecond)
	}

	// Check that the balances of each have updated appropriately, in
	// accordance with 0-conf.
	if origBal.Sub(consensus.NewCurrency64(15)).Cmp(dt.wallet.Balance(false)) != 0 {
		t.Error(origBal.Big())
		t.Error(dt.wallet.Balance(false).Big())
		t.Error("balances are incorrect for 0-conf transaction")
	}
	if origBal2.Add(consensus.NewCurrency64(15)).Cmp(dt2.wallet.Balance(false)) != 0 {
		t.Error(origBal2.Big())
		t.Error(dt2.wallet.Balance(false).Big())
		t.Error("balances are incorrect for 0-conf transaction")
	}
}

// TestBlockBootstrap checks that gateway.Synchronize will be effective even
// when the first state has a few thousand blocks.
func TestBlockBootstrap(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Create a daemon and give it 2500 blocks.
	dt := newDaemonTester(t)
	for i := 0; i < 2*gateway.MaxCatchUpBlocks+1; i++ {
		dt.mineBlock()
	}

	// Add a peer and spin until the peer is caught up. addPeer() already does
	// this check, but it's left here to be explict anyway.
	dt2 := dt.addPeer()
	for dt.state.Height() != dt2.state.Height() {
		time.Sleep(time.Millisecond)
	}
}
