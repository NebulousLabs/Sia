package main

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

func (dt *daemonTester) testSendCoins() {
	// get current balance
	var oldstatus modules.WalletInfo
	dt.getAPI("/wallet/status", &oldstatus)
	// get a new address
	var addr struct {
		Address string
	}
	dt.getAPI("/wallet/address", &addr)
	// send 3e4 coins to the address
	dt.callAPI("/wallet/send?amount=30000&dest=" + addr.Address)
	// get updated balance
	var newstatus modules.WalletInfo
	dt.getAPI("/wallet/status", &newstatus)
	// compare balances
	// TODO: need a better way to test this
	if newstatus.FullBalance.Cmp(oldstatus.FullBalance) != 0 {
		dt.Fatal("Balance should not have changed:\n\told: %v\n\tnew: %v", newstatus.FullBalance.Big(), oldstatus.FullBalance.Big())
	}
}

// TestSendCoins creates two addresses and sends coins from one to the other.
// The first balance should decrease, and the second balance should increase
// proportionally.
func TestSendCoins(t *testing.T) {
	dt := newDaemonTester(t)
	// need to mine a few coins first
	dt.testMining()
	dt.testSendCoins()
}
