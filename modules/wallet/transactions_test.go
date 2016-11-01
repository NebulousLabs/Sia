package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationTransactions checks that the transaction history is being
// correctly recorded and extended.
func TestIntegrationTransactions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester("TestIntegrationTransactions")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Creating the wallet tester results in blocks being mined until the miner
	// has money, which means types.MaturityDelay+1 blocks are created, and
	// each block is going to have a transaction (the miner payout) going to
	// the wallet.
	txns, err := wt.wallet.Transactions(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != int(types.MaturityDelay+1) {
		t.Error("unexpected transaction history length")
	}
	sendTxns, err := wt.wallet.SendSiacoins(types.NewCurrency64(5000), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	// No more confirmed transactions have been added.
	txns, err = wt.wallet.Transactions(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != int(types.MaturityDelay+1) {
		t.Error("unexpected transaction history length")
	}
	// Two transactions added to unconfirmed pool - 1 to fund the exact output,
	// and 1 to hold the exact output.
	if len(wt.wallet.UnconfirmedTransactions()) != 2 {
		t.Error("was expecting 4 unconfirmed transactions")
	}

	b, _ := wt.miner.FindBlock()
	err = wt.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	// A confirmed transaction was added for the miner payout, and the 2
	// transactions that were previously unconfirmed.
	txns, err = wt.wallet.Transactions(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(txns) != int(types.MaturityDelay+2+2) {
		t.Error("unexpected transaction history length")
	}

	// Try getting a partial history for just the previous block.
	txns, err = wt.wallet.Transactions(types.MaturityDelay+2, types.MaturityDelay+2)
	if err != nil {
		t.Fatal(err)
	}
	// The partial should include one transaction for a block, and 2 for the
	// send that occurred.
	if len(txns) != 3 {
		t.Error(len(txns))
	}

	// Check that transactions can be queried individually.
	txn, exists := wt.wallet.Transaction(sendTxns[0].ID())
	if !exists {
		t.Fatal("unable to query transaction")
	}
	if txn.TransactionID != sendTxns[0].ID() {
		t.Error("wrong transaction was fetched")
	}
	_, exists = wt.wallet.Transaction(types.TransactionID{})
	if exists {
		t.Error("able to query a nonexisting transction")
	}
}

// TestIntegrationAddressTransactions checks grabbing the history for a single
// address.
func TestIntegrationAddressTransactions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester("TestTransactionHistory")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Grab an address and send it money.
	uc, err := wt.wallet.NextAddress()
	addr := uc.UnlockHash()
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.wallet.SendSiacoins(types.NewCurrency64(5005), addr)
	if err != nil {
		t.Fatal(err)
	}

	// Check the confirmed balance of the address.
	addrHist := wt.wallet.AddressTransactions(addr)
	if len(addrHist) != 0 {
		t.Error("address should be empty - no confirmed transactions")
	}
	if len(wt.wallet.AddressUnconfirmedTransactions(addr)) == 0 {
		t.Error("addresses unconfirmed transactions should not be empty")
	}
	b, _ := wt.miner.FindBlock()
	err = wt.cs.AcceptBlock(b)
	if err != nil {
		t.Fatal(err)
	}
	addrHist = wt.wallet.AddressTransactions(addr)
	if len(addrHist) == 0 {
		t.Error("address history should have some transactions")
	}
	if len(wt.wallet.AddressUnconfirmedTransactions(addr)) != 0 {
		t.Error("addresses unconfirmed transactions should be empty")
	}
}
