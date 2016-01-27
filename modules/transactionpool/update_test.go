package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestArbDataOnly tries submitting a transaction with only aribtrary data to
// the transaction pool. Then a block is mined, putting the transaction on the
// blockchain. The arb data transaction should no longer be in the transaction
// pool.
func TestArbDataOnly(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester("TestArbDataOnly")
	if err != nil {
		t.Fatal(err)
	}
	txn := types.Transaction{
		ArbitraryData: [][]byte{
			append(modules.PrefixNonSia[:], []byte("arb-data")...),
		},
	}
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{txn})
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.TransactionList()) != 1 {
		t.Error("expecting to see a transaction in the transaction pool")
	}
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.TransactionList()) != 0 {
		t.Error("transaction was not cleared from the transaction pool")
	}
}
