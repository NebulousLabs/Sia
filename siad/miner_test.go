package main

import (
	"fmt"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/miner"
)

// TestMining starts the miner, mines a few blocks, and checks that the wallet
// balance increased.
func TestMining(t *testing.T) {
	dt := newDaemonTester(t)
	// start miner
	dt.callAPI("/miner/start?threads=1")
	// check that miner has started
	var minerstatus miner.MinerInfo
	dt.getAPI("/miner/status", &minerstatus)
	fmt.Println(minerstatus)
	if minerstatus.State != "On" {
		dt.Fatal("Miner did not start")
	}
	dt.callAPI("/miner/stop")
	// check balance
	var walletstatus modules.WalletInfo
	dt.getAPI("/wallet/status", &walletstatus)
	if walletstatus.Balance.Sign() <= 0 {
		dt.Fatal("Mining did not increase wallet balance")
	}
}
