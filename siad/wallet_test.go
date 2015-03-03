package main

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
)

// TestSendCoins creates two addresses and sends coins from one to the other.
// The first balance should decrease, and the second balance should increase
// proportionally.
func TestSendCoins(t *testing.T) {
	sender := newDaemonTester(t)
	receiver := sender.addPeer()

	// Give some money to the sender.
	sender.mineMoney()

	// get current balances
	var oldSenderStatus modules.WalletInfo
	sender.getAPI("/wallet/status", &oldSenderStatus)
	var oldReceiverStatus modules.WalletInfo
	receiver.getAPI("/wallet/status", &oldReceiverStatus)

	// send 3 coins from the sender to the receiver
	sender.callAPI("/wallet/send?amount=3&dest=" + receiver.coinAddress())
	// wait until the transaction is relayed to the receiver
	<-receiver.rpcChan
	<-receiver.rpcChan
	//<-sender.rpcChan
	//<-sender.rpcChan

	// get updated balances
	var newSenderStatus modules.WalletInfo
	sender.getAPI("/wallet/status", &newSenderStatus)
	var newReceiverStatus modules.WalletInfo
	receiver.getAPI("/wallet/status", &newReceiverStatus)

	// sender balance should have gone down
	if newSenderStatus.FullBalance.Cmp(oldSenderStatus.FullBalance) >= 0 {
		t.Fatalf("Sender balance should have gone down:\n\told: %v\n\tnew: %v", oldSenderStatus.FullBalance.Big(), newSenderStatus.FullBalance.Big())
	}
	// receiver balance should have gone up
	if newReceiverStatus.FullBalance.Cmp(oldReceiverStatus.FullBalance) <= 0 {
		t.Fatalf("Receiver balance should have gone up:\n\told: %v\n\tnew: %v", oldReceiverStatus.FullBalance.Big(), newReceiverStatus.FullBalance.Big())
	}
}
