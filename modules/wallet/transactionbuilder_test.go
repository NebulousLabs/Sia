package wallet

/*
import (
	"errors"
	"testing"

	"github.com/NebulousLabs/Sia/types"
)

// testFundTransaction funds and completes a transaction using the
// build-your-own transaction functions, checking that a no-refund transaction
// is created that is valid.
func (wt *walletTester) testFundTransaction() error {
	// Build a transaction that intentionally needs a refund.
	txnBuilder := wt.wallet.StartTransaction()
	fund := wt.wallet.Balance(false).Sub(types.NewCurrency64(1))
	err := txnBuilder.FundSiacoins(fund)
	if err != nil {
		return err
	}
	txnBuilder.AddMinerFee(fund)
	txn, parents := txnBuilder.View()
	txnSet := append(parents, txn)
	err = wt.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return err
	}

	// Check that the length of the created transaction is 1 siacoin, and that
	// the unconfirmed balance of the wallet is 1.
	if len(txnSet[len(txnSet)-1].SiacoinOutputs) != 0 {
		return errors.New("expecting 0 siacoin outputs, got non-zero result")
	}
	if wt.wallet.Balance(true).Cmp(types.NewCurrency64(1)) != 0 {
		return errors.New("incorrect balance being reported")
	}

	// Dump the transaction pool into a block and see that the balance still
	// registers correctly.
	b, _ := wt.miner.FindBlock()
	err = wt.cs.AcceptBlock(b)
	if err != nil {
		return err
	}

	// Check that the length of the created transaction is 1 siacoin, and that
	// the unconfirmed balance of the wallet is 1 + BlockReward.
	if len(txnSet[len(txnSet)-1].SiacoinOutputs) != 0 {
		return errors.New("wrong number of siacoin outputs - expecting 0")
	}
	expectedBalance := types.CalculateCoinbase(2).Add(types.NewCurrency64(1))
	if bal := wt.wallet.Balance(true); bal.Cmp(expectedBalance) != 0 {
		return errors.New("did not arrive at the expected balance")
	}
	return nil
}

// TestFundTransaction creates a wallet tester and uses it to call
// testFundTransaction.
func TestFundTransaction(t *testing.T) {
	t.Skip("wallet is still borked")
	wt, err := createWalletTester("TestFundTransaction")
	if err != nil {
		t.Fatal(err)
	}
	err = wt.testFundTransaction()
	if err != nil {
		t.Error(err)
	}
}
*/
