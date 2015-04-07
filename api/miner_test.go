package api

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

// TestMining starts the miner, mines a few blocks, and checks that the wallet
// balance increased.
func TestMining(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	st := newServerTester("TestMining", t)
	// start miner
	st.callAPI("/miner/start?threads=1")
	// check that miner has started
	var minerstatus modules.MinerInfo
	st.getAPI("/miner/status", &minerstatus)
	if minerstatus.State != "On" {
		st.Fatal("Miner did not start")
	}
	time.Sleep(1000 * time.Millisecond)
	st.callAPI("/miner/stop")
	// check balance
	var walletstatus modules.WalletInfo
	st.getAPI("/wallet/status", &walletstatus)
	if walletstatus.FullBalance.IsZero() {
		st.Fatalf("Mining did not increase wallet balance: %v", walletstatus.FullBalance.Big())
	}
}
