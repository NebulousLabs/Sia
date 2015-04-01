package api

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

func (dt *daemonTester) testMining() {
	if testing.Short() {
		dt.Skip()
	}

	// start miner
	dt.callAPI("/miner/start?threads=1")
	// check that miner has started
	var minerstatus modules.MinerInfo
	dt.getAPI("/miner/status", &minerstatus)
	if minerstatus.State != "On" {
		dt.Fatal("Miner did not start")
	}
	time.Sleep(1000 * time.Millisecond)
	dt.callAPI("/miner/stop")
	// check balance
	var walletstatus modules.WalletInfo
	dt.getAPI("/wallet/status", &walletstatus)
	if walletstatus.FullBalance.Sign() <= 0 {
		dt.Fatalf("Mining did not increase wallet balance: %v", walletstatus.FullBalance.Big())
	}
}

// TestMining starts the miner, mines a few blocks, and checks that the wallet
// balance increased.
func TestMining(t *testing.T) {
	dt := newDaemonTester(t)
	dt.testMining()
}
