package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestIntegrationTransactions checks that the transaction history is being
// correctly recorded and extended.
func TestIntegrationTransactions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name())
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
	sentValue := types.NewCurrency64(5000)
	_, err = wt.wallet.SendSiacoins(sentValue, types.UnlockHash{})
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
}

// TestIntegrationTransaction checks that individually queried transactions
// contain the correct values.
func TestIntegrationTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	_, exists := wt.wallet.Transaction(types.TransactionID{})
	if exists {
		t.Error("able to query a nonexisting transction")
	}

	// test sending siacoins
	sentValue := types.NewCurrency64(5000)
	sendTxns, err := wt.wallet.SendSiacoins(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// sendTxns[0] is the set-up transaction, sendTxns[1] contains the sentValue output
	txn, exists := wt.wallet.Transaction(sendTxns[1].ID())
	if !exists {
		t.Fatal("unable to query transaction")
	}
	if txn.TransactionID != sendTxns[1].ID() {
		t.Error("wrong transaction was fetched")
	} else if len(txn.Inputs) != 1 || len(txn.Outputs) != 2 {
		t.Error("expected 1 input and 2 outputs, got", len(txn.Inputs), len(txn.Outputs))
	} else if !txn.Outputs[0].Value.Equals(sentValue) {
		t.Errorf("expected first output to equal %v, got %v", sentValue, txn.Outputs[0].Value)
	} else if exp := txn.Inputs[0].Value.Sub(sentValue); !txn.Outputs[1].Value.Equals(exp) {
		t.Errorf("expected first output to equal %v, got %v", exp, txn.Outputs[1].Value)
	}

	// test sending siafunds
	err = wt.wallet.LoadSiagKeys(wt.walletMasterKey, []string{"../../types/siag0of1of1.siakey"})
	if err != nil {
		t.Error(err)
	}
	sentValue = types.NewCurrency64(12)
	sendTxns, err = wt.wallet.SendSiafunds(sentValue, types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	txn, exists = wt.wallet.Transaction(sendTxns[1].ID())
	if !exists {
		t.Fatal("unable to query transaction")
	}
	if len(txn.Inputs) != 1 || len(txn.Outputs) != 3 {
		t.Error("expected 1 input and 3 outputs, got", len(txn.Inputs), len(txn.Outputs))
	} else if !txn.Outputs[1].Value.Equals(sentValue) {
		t.Errorf("expected first output to equal %v, got %v", sentValue, txn.Outputs[1].Value)
	} else if exp := txn.Inputs[0].Value.Sub(sentValue); !txn.Outputs[2].Value.Equals(exp) {
		t.Errorf("expected first output to equal %v, got %v", exp, txn.Outputs[2].Value)
	}
}

// TestIntegrationAddressTransactions checks grabbing the history for a single
// address.
func TestIntegrationAddressTransactions(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name())
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

// TestTransactionInputOutputIDs verifies that ProcessedTransaction's inputs
// and outputs have a valid ID field.
func TestTransactionInputOutputIDs(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// mine a few blocks to create miner payouts
	for i := 0; i < 5; i++ {
		_, err = wt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// create some siacoin outputs
	uc, err := wt.wallet.NextAddress()
	addr := uc.UnlockHash()
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.wallet.SendSiacoins(types.NewCurrency64(5005), addr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// verify the miner payouts and siacoin outputs/inputs have correct IDs
	txns, err := wt.wallet.Transactions(0, 1000)
	if err != nil {
		t.Fatal(err)
	}

	outputIDs := make(map[types.OutputID]struct{})
	for _, txn := range txns {
		block, _ := wt.cs.BlockAtHeight(txn.ConfirmationHeight)
		for i, output := range txn.Outputs {
			outputIDs[output.ID] = struct{}{}
			if output.FundType == types.SpecifierMinerPayout {
				if output.ID != types.OutputID(block.MinerPayoutID(uint64(i))) {
					t.Fatal("miner payout had incorrect output ID")
				}
			}
			if output.FundType == types.SpecifierSiacoinOutput {
				if output.ID != types.OutputID(txn.Transaction.SiacoinOutputID(uint64(i))) {
					t.Fatal("siacoin output had incorrect output ID")
				}
			}
		}
		for _, input := range txn.Inputs {
			if _, exists := outputIDs[input.ParentID]; !exists {
				t.Fatal("input has ParentID that points to a nonexistent output:", input.ParentID)
			}
		}
	}
}

// TestWalletTransactionsContext verifies that transactions correctly set their
// `context` field.
func TestWalletTransactionsContext(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	wt.wallet.SetContextLimit("TestContext", types.SiacoinPrecision.Mul64(100))
	output := types.SiacoinOutput{
		Value:      types.SiacoinPrecision.Mul64(50),
		UnlockHash: types.UnlockHash{},
	}
	txnBuilder := wt.wallet.StartTransaction()
	txnBuilder.SetContext("TestContext")
	err = txnBuilder.FundSiacoins(types.SiacoinPrecision.Mul64(50))
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddSiacoinOutput(output)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = wt.wallet.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	txns, err := wt.wallet.Transactions(0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var contextTxns []modules.ProcessedTransaction
	for _, txn := range txns {
		if txn.Context == "TestContext" {
			contextTxns = append(contextTxns, txn)
		}
	}
	if len(contextTxns) != 1 {
		t.Fatal("expected one matching contextual transactions")
	}
	val := types.NewCurrency64(0)
	for _, output := range contextTxns[0].Outputs {
		if output.FundType == types.SpecifierSiacoinOutput {
			val = val.Add(output.Value)
		}
	}
	if val.Cmp(types.SiacoinPrecision.Mul64(50)) != 0 {
		t.Fatal("expected contextual transaction list to return our 50SC transaction")
	}
}
