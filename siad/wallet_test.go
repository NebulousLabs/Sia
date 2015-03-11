package main

import (
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/modules"
)

// TestSendCoins creates two addresses and sends coins from one to the other.
// The first balance should decrease, and the second balance should increase
// proportionally.
func TestSendCoins(t *testing.T) {
	sender := newDaemonTester(t)
	receiver := sender.addPeer()

	// get current balances
	var oldSenderStatus modules.WalletInfo
	sender.getAPI("/wallet/status", &oldSenderStatus)
	var oldReceiverStatus modules.WalletInfo
	receiver.getAPI("/wallet/status", &oldReceiverStatus)

	// send 3 coins from the sender to the receiver
	sender.callAPI("/wallet/send?amount=3&destination=" + receiver.coinAddress())

	// get updated balances
	var newSenderStatus modules.WalletInfo
	sender.getAPI("/wallet/status", &newSenderStatus)
	var newReceiverStatus modules.WalletInfo
	receiver.getAPI("/wallet/status", &newReceiverStatus)

	// sender balance should have gone down
	for newSenderStatus.Balance.Cmp(oldSenderStatus.Balance) >= 0 {
		// t.Fatalf("Sender balance should have gone down:\n\told: %v\n\tnew: %v", oldSenderStatus.Balance.Big(), newSenderStatus.Balance.Big())
		time.Sleep(time.Millisecond)
	}
	// receiver balance should have gone up
	for newReceiverStatus.Balance.Cmp(oldReceiverStatus.Balance) <= 0 {
		// t.Fatalf("Receiver balance should have gone up:\n\told: %v\n\tnew: %v", oldReceiverStatus.Balance.Big(), newReceiverStatus.Balance.Big())
		time.Sleep(time.Millisecond)
	}
}
