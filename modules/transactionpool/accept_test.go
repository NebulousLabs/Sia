package transactionpool

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"
)

// TestIntegrationAcceptTransactionSet probes the AcceptTransactionSet method
// of the transaction pool.
func TestIntegrationAcceptTransactionSet(t *testing.T) {
	// Create a transaction pool tester.
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Check that the transaction pool is empty.
	if len(tpt.tpool.transactionSets) != 0 {
		t.Error("transaction pool is not empty")
	}

	// Create a valid transaction set using the wallet.
	txns, err := tpt.wallet.SendSiacoins(types.NewCurrency64(100), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.transactionSets) != 1 {
		t.Error("sending coins did not increase the transaction sets by 1")
	}

	// Submit the transaction set again to trigger a duplication error.
	err = tpt.tpool.AcceptTransactionSet(txns)
	if err != modules.ErrDuplicateTransactionSet {
		t.Error(err)
	}

	// Mine a block and check that the transaction pool gets emptied.
	block, _ := tpt.miner.FindBlock()
	err = tpt.cs.AcceptBlock(block)
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.TransactionList()) != 0 {
		t.Error("transaction pool was not emptied after mining a block")
	}

	// Try to resubmit the transaction set to verify
	err = tpt.tpool.AcceptTransactionSet(txns)
	if err == nil {
		t.Error("transaction set was supposed to be rejected")
	}
}

// TestIntegrationConflictingTransactionSets tries to add two transaction sets
// to the transaction pool that are each legal individually, but double spend
// an output.
func TestIntegrationConflictingTransactionSets(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a transaction pool tester.
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Fund a partial transaction.
	fund := types.NewCurrency64(30e6)
	txnBuilder := tpt.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fund)
	if err != nil {
		t.Fatal(err)
	}
	// wholeTransaction is set to false so that we can use the same signature
	// to create a double spend.
	txnSet, err := txnBuilder.Sign(false)
	if err != nil {
		t.Fatal(err)
	}
	txnSetDoubleSpend := make([]types.Transaction, len(txnSet))
	copy(txnSetDoubleSpend, txnSet)

	// There are now two sets of transactions that are signed and ready to
	// spend the same output. Have one spend the money in a miner fee, and the
	// other create a siacoin output.
	txnIndex := len(txnSet) - 1
	txnSet[txnIndex].MinerFees = append(txnSet[txnIndex].MinerFees, fund)
	txnSetDoubleSpend[txnIndex].SiacoinOutputs = append(txnSetDoubleSpend[txnIndex].SiacoinOutputs, types.SiacoinOutput{Value: fund})

	// Add the first and then the second txn set.
	err = tpt.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Error(err)
	}
	err = tpt.tpool.AcceptTransactionSet(txnSetDoubleSpend)
	if err == nil {
		t.Error("transaction should not have passed inspection")
	}

	// Purge and try the sets in the reverse order.
	tpt.tpool.PurgeTransactionPool()
	err = tpt.tpool.AcceptTransactionSet(txnSetDoubleSpend)
	if err != nil {
		t.Error(err)
	}
	err = tpt.tpool.AcceptTransactionSet(txnSet)
	if err == nil {
		t.Error("transaction should not have passed inspection")
	}
}

// TestIntegrationCheckMinerFees probes the checkMinerFees method of the
// transaction pool.
func TestIntegrationCheckMinerFees(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a transaction pool tester.
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Fill the transaction pool to the fee limit.
	for i := 0; i < TransactionPoolSizeForFee/10e3; i++ {
		arbData := make([]byte, 10e3)
		copy(arbData, modules.PrefixNonSia[:])
		fastrand.Read(arbData[100:116]) // prevents collisions with other transacitons in the loop.
		txn := types.Transaction{ArbitraryData: [][]byte{arbData}}
		err := tpt.tpool.AcceptTransactionSet([]types.Transaction{txn})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Add another transaction, this one should fail for having too few fees.
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{{}})
	if err != errLowMinerFees {
		t.Error(err)
	}

	// Add a transaction that has sufficient fees.
	_, err = tpt.wallet.SendSiacoins(types.NewCurrency64(100), types.UnlockHash{})
	if err != nil {
		t.Error(err)
	}

	// TODO: fill the pool up all the way and try again.
}

// TestTransactionSuperset submits a single transaction to the network,
// followed by a transaction set containing that single transaction.
func TestIntegrationTransactionSuperset(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a transaction pool tester.
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Fund a partial transaction.
	fund := types.NewCurrency64(30e6)
	txnBuilder := tpt.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fund)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddMinerFee(fund)
	// wholeTransaction is set to false so that we can use the same signature
	// to create a double spend.
	txnSet, err := txnBuilder.Sign(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(txnSet) <= 1 {
		t.Fatal("test is invalid unless the transaction set has two or more transactions")
	}
	// Check that the second transaction is dependent on the first.
	err = tpt.tpool.AcceptTransactionSet(txnSet[1:])
	if err == nil {
		t.Fatal("transaction set must have dependent transactions")
	}

	// Submit the first transaction in the set to the transaction pool, and
	// then the superset.
	err = tpt.tpool.AcceptTransactionSet(txnSet[:1])
	if err != nil {
		t.Fatal("first transaction in the transaction set was not valid?")
	}
	err = tpt.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal("super setting is not working:", err)
	}

	// Try resubmitting the individual transaction and the superset, a
	// duplication error should be returned for each case.
	err = tpt.tpool.AcceptTransactionSet(txnSet[:1])
	if err != modules.ErrDuplicateTransactionSet {
		t.Fatal(err)
	}
	err = tpt.tpool.AcceptTransactionSet(txnSet)
	if err != modules.ErrDuplicateTransactionSet {
		t.Fatal("super setting is not working:", err)
	}
}

// TestTransactionSubset submits a transaction set to the network, followed by
// just a subset, expectint ErrDuplicateTransactionSet as a response.
func TestIntegrationTransactionSubset(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a transaction pool tester.
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Fund a partial transaction.
	fund := types.NewCurrency64(30e6)
	txnBuilder := tpt.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fund)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddMinerFee(fund)
	// wholeTransaction is set to false so that we can use the same signature
	// to create a double spend.
	txnSet, err := txnBuilder.Sign(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(txnSet) <= 1 {
		t.Fatal("test is invalid unless the transaction set has two or more transactions")
	}
	// Check that the second transaction is dependent on the first.
	err = tpt.tpool.AcceptTransactionSet(txnSet[1:])
	if err == nil {
		t.Fatal("transaction set must have dependent transactions")
	}

	// Submit the set to the pool, followed by just the transaction.
	err = tpt.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal("super setting is not working:", err)
	}
	err = tpt.tpool.AcceptTransactionSet(txnSet[:1])
	if err != modules.ErrDuplicateTransactionSet {
		t.Fatal(err)
	}
}

// TestIntegrationTransactionChild submits a single transaction to the network,
// followed by a child transaction.
func TestIntegrationTransactionChild(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a transaction pool tester.
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Fund a partial transaction.
	fund := types.NewCurrency64(30e6)
	txnBuilder := tpt.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fund)
	if err != nil {
		t.Fatal(err)
	}
	txnBuilder.AddMinerFee(fund)
	// wholeTransaction is set to false so that we can use the same signature
	// to create a double spend.
	txnSet, err := txnBuilder.Sign(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(txnSet) <= 1 {
		t.Fatal("test is invalid unless the transaction set has two or more transactions")
	}
	// Check that the second transaction is dependent on the first.
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{txnSet[1]})
	if err == nil {
		t.Fatal("transaction set must have dependent transactions")
	}

	// Submit the first transaction in the set to the transaction pool.
	err = tpt.tpool.AcceptTransactionSet(txnSet[:1])
	if err != nil {
		t.Fatal("first transaction in the transaction set was not valid?")
	}
	err = tpt.tpool.AcceptTransactionSet(txnSet[1:])
	if err != nil {
		t.Fatal("child transaction not seen as valid")
	}
}

// TestIntegrationNilAccept tries submitting a nil transaction set and a 0-len
// transaction set to the transaction pool.
func TestIntegrationNilAccept(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	err = tpt.tpool.AcceptTransactionSet(nil)
	if err == nil {
		t.Error("no error returned when submitting nothing to the transaction pool")
	}
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{})
	if err == nil {
		t.Error("no error returned when submitting nothing to the transaction pool")
	}
}

// TestAcceptFCAndConflictingRevision checks that the transaction pool
// correctly accepts a file contract in a transaction set followed by a correct
// revision to that file contract in the a following transaction set, with no
// block separating them.
func TestAcceptFCAndConflictingRevision(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Create and fund a valid file contract.
	builder := tpt.wallet.StartTransaction()
	payout := types.NewCurrency64(1e9)
	err = builder.FundSiacoins(payout)
	if err != nil {
		t.Fatal(err)
	}
	builder.AddFileContract(types.FileContract{
		WindowStart:        tpt.cs.Height() + 2,
		WindowEnd:          tpt.cs.Height() + 5,
		Payout:             payout,
		ValidProofOutputs:  []types.SiacoinOutput{{Value: types.PostTax(tpt.cs.Height(), payout)}},
		MissedProofOutputs: []types.SiacoinOutput{{Value: types.PostTax(tpt.cs.Height(), payout)}},
		UnlockHash:         types.UnlockConditions{}.UnlockHash(),
	})
	tSet, err := builder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = tpt.tpool.AcceptTransactionSet(tSet)
	if err != nil {
		t.Fatal(err)
	}
	fcid := tSet[len(tSet)-1].FileContractID(0)

	// Create a file contract revision and submit it.
	rSet := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          fcid,
			NewRevisionNumber: 2,

			NewWindowStart:        tpt.cs.Height() + 2,
			NewWindowEnd:          tpt.cs.Height() + 5,
			NewValidProofOutputs:  []types.SiacoinOutput{{Value: types.PostTax(tpt.cs.Height(), payout)}},
			NewMissedProofOutputs: []types.SiacoinOutput{{Value: types.PostTax(tpt.cs.Height(), payout)}},
		}},
	}}
	err = tpt.tpool.AcceptTransactionSet(rSet)
	if err != nil {
		t.Fatal(err)
	}
}

// TestPartialConfirmation checks that the transaction pool correctly accepts a
// transaction set which has parents that have been accepted by the consensus
// set but not the whole set has been accepted by the consensus set.
func TestPartialConfirmation(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Create and fund a valid file contract.
	builder := tpt.wallet.StartTransaction()
	payout := types.NewCurrency64(1e9)
	err = builder.FundSiacoins(payout)
	if err != nil {
		t.Fatal(err)
	}
	builder.AddFileContract(types.FileContract{
		WindowStart:        tpt.cs.Height() + 2,
		WindowEnd:          tpt.cs.Height() + 5,
		Payout:             payout,
		ValidProofOutputs:  []types.SiacoinOutput{{Value: types.PostTax(tpt.cs.Height(), payout)}},
		MissedProofOutputs: []types.SiacoinOutput{{Value: types.PostTax(tpt.cs.Height(), payout)}},
		UnlockHash:         types.UnlockConditions{}.UnlockHash(),
	})
	tSet, err := builder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	fcid := tSet[len(tSet)-1].FileContractID(0)

	// Create a file contract revision.
	rSet := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          fcid,
			NewRevisionNumber: 2,

			NewWindowStart:        tpt.cs.Height() + 2,
			NewWindowEnd:          tpt.cs.Height() + 5,
			NewValidProofOutputs:  []types.SiacoinOutput{{Value: types.PostTax(tpt.cs.Height(), payout)}},
			NewMissedProofOutputs: []types.SiacoinOutput{{Value: types.PostTax(tpt.cs.Height(), payout)}},
		}},
	}}

	// Combine the contract and revision in to a single set.
	fullSet := append(tSet, rSet...)

	// Get the tSet onto the blockchain.
	unsolvedBlock, target, err := tpt.miner.BlockForWork()
	if err != nil {
		t.Fatal(err)
	}
	unsolvedBlock.Transactions = append(unsolvedBlock.Transactions, tSet...)
	solvedBlock, solved := tpt.miner.SolveBlock(unsolvedBlock, target)
	if !solved {
		t.Fatal("Failed to solve block")
	}
	err = tpt.cs.AcceptBlock(solvedBlock)
	if err != nil {
		t.Fatal(err)
	}

	// Try to get the full set into the transaction pool. The transaction pool
	// should recognize that the set is partially accepted, and be able to
	// accept on the the transactions that are new and are not yet on the
	// blockchain.
	err = tpt.tpool.AcceptTransactionSet(fullSet)
	if err != nil {
		t.Fatal(err)
	}
}

// TestPartialConfirmationWeave checks that the transaction pool correctly
// accepts a transaction set which has parents that have been accepted by the
// consensus set but not the whole set has been accepted by the consensus set,
// this time weaving the dependencies, such that the first transaction is not
// in the consensus set, the second is, and the third has both as dependencies.
func TestPartialConfirmationWeave(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Step 1: create an output to the empty address in a tx.
	// Step 2: create a second output to the empty address in another tx.
	// Step 3: create a transaction using both those outputs.
	// Step 4: mine the txn set in step 2
	// Step 5: Submit the complete set.

	// Create a transaction with a single output to a fully controlled address.
	emptyUH := types.UnlockConditions{}.UnlockHash()
	builder1 := tpt.wallet.StartTransaction()
	funding1 := types.NewCurrency64(1e9)
	err = builder1.FundSiacoins(funding1)
	if err != nil {
		t.Fatal(err)
	}
	scOutput1 := types.SiacoinOutput{
		Value:      funding1,
		UnlockHash: emptyUH,
	}
	i1 := builder1.AddSiacoinOutput(scOutput1)
	tSet1, err := builder1.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	// Submit to the transaction pool and mine the block, to minimize
	// complexity.
	err = tpt.tpool.AcceptTransactionSet(tSet1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Create a second output to the fully controlled address, to fund the
	// second transaction in the weave.
	builder2 := tpt.wallet.StartTransaction()
	funding2 := types.NewCurrency64(2e9)
	err = builder2.FundSiacoins(funding2)
	if err != nil {
		t.Fatal(err)
	}
	scOutput2 := types.SiacoinOutput{
		Value:      funding2,
		UnlockHash: emptyUH,
	}
	i2 := builder2.AddSiacoinOutput(scOutput2)
	tSet2, err := builder2.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	// Submit to the transaction pool and mine the block, to minimize
	// complexity.
	err = tpt.tpool.AcceptTransactionSet(tSet2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Create a passthrough transaction for output1 and output2, so that they
	// can be used as unconfirmed dependencies.
	txn1 := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID: tSet1[len(tSet1)-1].SiacoinOutputID(i1),
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      funding1,
			UnlockHash: emptyUH,
		}},
	}
	txn2 := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID: tSet2[len(tSet2)-1].SiacoinOutputID(i2),
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      funding2,
			UnlockHash: emptyUH,
		}},
	}

	// Create a child transaction that depends on inputs from both txn1 and
	// txn2.
	child := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			{
				ParentID: txn1.SiacoinOutputID(0),
			},
			{
				ParentID: txn2.SiacoinOutputID(0),
			},
		},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value: funding1.Add(funding2),
		}},
	}

	// Get txn2 accepted into the consensus set.
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{txn2})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Try to get the set of txn1, txn2, and child accepted into the
	// transaction pool.
	err = tpt.tpool.AcceptTransactionSet([]types.Transaction{txn1, txn2, child})
	if err != nil {
		t.Fatal(err)
	}
}
