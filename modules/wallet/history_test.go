package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestTransactionHistory checks that the transaction history is being
// correctly recorded and extended.
func TestTransactionHistory(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	wt, err := createWalletTester("TestTransactionHistory")
	if err != nil {
		t.Fatal(err)
	}
	// Creating the wallet tester results in blocks being mined until the miner
	// has money, which means types.MaturityDelay+1 blocks are created, and
	// each block is going to have a transaction (the miner payout) going to
	// the wallet.
	if len(wt.wallet.ConfirmedTransactionHistory()) != int(types.MaturityDelay+1) {
		t.Error("unexpected transaction history length")
	}
	_, err = wt.wallet.SendSiacoins(types.NewCurrency64(5000), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	// No more confirmed transactions have been added.
	if len(wt.wallet.ConfirmedTransactionHistory()) != int(types.MaturityDelay+1) {
		t.Error("unexpected transaction history length")
	}
	// Four transactions were added: to fund the parent txn (an input), create
	// an exact output (an output) for the child, and refund the parent (an
	// output), and then an input to the child transaction (an input). The
	// output of the child transaction is not tracked by the wallet.
	if len(wt.wallet.UnconfirmedTransactions()) != 4 {
		t.Error("was expecting 4 unconfirmed transactions")
	}

	b, _ := wt.miner.FindBlock()
	err = wt.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	// A confirmed transaction was added for the miner payout, and the 4
	// transactions that were previously unconfirmed.
	if len(wt.wallet.ConfirmedTransactionHistory()) != int(types.MaturityDelay+2+4) {
		t.Error("unexpected transaction history length")
	}

	// Try getting a partial history for just the previous block.
	txns, err := wt.wallet.PartialTransactionHistory(types.MaturityDelay+3, types.MaturityDelay+3)
	if err != nil {
		t.Fatal(err)
	}
	// The partial should include one transaction for a block, and 4 for the
	// send that occured.
	if len(txns) != 5 {
		t.Error(len(txns))
	}
}
