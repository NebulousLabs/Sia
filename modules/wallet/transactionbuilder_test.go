package wallet

import (
	"sync"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// addBlockNoPayout adds a block to the wallet tester that does not have any
// payouts.
func (wt *walletTester) addBlockNoPayout() error {
	block, target, err := wt.miner.BlockForWork()
	if err != nil {
		return err
	}
	// Clear the miner payout so that the wallet is not getting additional
	// outputs from these blocks.
	for i := range block.MinerPayouts {
		block.MinerPayouts[i].UnlockHash = types.UnlockHash{}
	}

	// Solve and submit the block.
	solvedBlock, _ := wt.miner.SolveBlock(block, target)
	err = wt.cs.AcceptBlock(solvedBlock)
	if err != nil {
		return err
	}
	return nil
}

// TestViewAdded checks that 'ViewAdded' returns sane-seeming values when
// indicating which elements have been added automatically to a transaction
// set.
func TestViewAdded(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Mine an extra block to get more outputs - the wallet is going to be
	// loading two transactions at the same time.
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Create a transaction, add money to it, spend the money in a miner fee
	// but do not sign the transaction. The format of this test mimics the way
	// that the host-renter protocol behaves when building a file contract
	// transaction.
	b := wt.wallet.StartTransaction()
	txnFund := types.NewCurrency64(100e9)
	err = b.FundSiacoins(txnFund)
	if err != nil {
		t.Fatal(err)
	}
	_ = b.AddMinerFee(txnFund)
	_ = b.AddSiacoinOutput(types.SiacoinOutput{Value: txnFund})
	unfinishedTxn, unfinishedParents := b.View()

	// Create a second builder that extends the first, unsigned transaction. Do
	// not sign the transaction, but do give the extensions to the original
	// builder.
	b2 := wt.wallet.RegisterTransaction(unfinishedTxn, unfinishedParents)
	err = b2.FundSiacoins(txnFund)
	if err != nil {
		t.Fatal(err)
	}
	unfinishedTxn2, unfinishedParents2 := b2.View()
	newParentIndices, newInputIndices, _, _ := b2.ViewAdded()

	// Add the new elements from b2 to b and sign the transaction, fetching the
	// signature for b.
	for _, parentIndex := range newParentIndices {
		b.AddParents([]types.Transaction{unfinishedParents2[parentIndex]})
	}
	for _, inputIndex := range newInputIndices {
		b.AddSiacoinInput(unfinishedTxn2.SiacoinInputs[inputIndex])
	}
	// Signing with WholeTransaction=true makes the transaction more brittle to
	// construction mistakes, meaning that an error is more likely to turn up.
	set1, err := b.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	if set1[len(set1)-1].ID() == unfinishedTxn.ID() {
		t.Error("seems like there's memory sharing happening between txn calls")
	}
	// Set1 should be missing some signatures.
	err = wt.tpool.AcceptTransactionSet(set1)
	if err == nil {
		t.Fatal(err)
	}
	unfinishedTxn3, _ := b.View()
	// Only the new signatures are needed because the previous call to 'View'
	// included everything else.
	_, _, _, newTxnSignaturesIndices := b.ViewAdded()

	// Add the new signatures to b2, and then sign b2's inputs. The resulting
	// set from b2 should be valid.
	for _, sigIndex := range newTxnSignaturesIndices {
		b2.AddTransactionSignature(unfinishedTxn3.TransactionSignatures[sigIndex])
	}
	set2, err := b2.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = wt.tpool.AcceptTransactionSet(set2)
	if err != nil {
		t.Fatal(err)
	}
	finishedTxn, _ := b2.View()
	_, _, _, newTxnSignaturesIndices3 := b2.ViewAdded()

	// Add the new signatures from b2 to the b1 transaction, which should
	// complete the transaction and create a transaction set in 'b' that is
	// identical to the transaction set that is in b2.
	for _, sigIndex := range newTxnSignaturesIndices3 {
		b.AddTransactionSignature(finishedTxn.TransactionSignatures[sigIndex])
	}
	set3Txn, set3Parents := b.View()
	err = wt.tpool.AcceptTransactionSet(append(set3Parents, set3Txn))
	if err != modules.ErrDuplicateTransactionSet {
		t.Fatal(err)
	}
}

// TestDoubleSignError checks that an error is returned if there is a problem
// when trying to call 'Sign' on a transaction twice.
func TestDoubleSignError(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Create a transaction, add money to it, and then call sign twice.
	b := wt.wallet.StartTransaction()
	txnFund := types.NewCurrency64(100e9)
	err = b.FundSiacoins(txnFund)
	if err != nil {
		t.Fatal(err)
	}
	_ = b.AddMinerFee(txnFund)
	txnSet, err := b.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	txnSet2, err := b.Sign(true)
	if err != errBuilderAlreadySigned {
		t.Error("the wrong error is being returned after a double call to sign")
	}
	if err != nil && txnSet2 != nil {
		t.Error("errored call to sign did not return a nil txn set")
	}
	err = wt.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		t.Fatal(err)
	}
}

// TestConcurrentBuilders checks that multiple transaction builders can safely
// be opened at the same time, and that they will make valid transactions when
// building concurrently.
func TestConcurrentBuilders(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Mine a few more blocks so that the wallet has lots of outputs to pick
	// from.
	for i := 0; i < 5; i++ {
		_, err := wt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Get a baseline balance for the wallet.
	startingSCConfirmed, _, _ := wt.wallet.ConfirmedBalance()
	startingOutgoing, startingIncoming := wt.wallet.UnconfirmedBalance()
	if !startingOutgoing.IsZero() {
		t.Fatal(startingOutgoing)
	}
	if !startingIncoming.IsZero() {
		t.Fatal(startingIncoming)
	}

	// Create two builders at the same time, then add money to each.
	builder1 := wt.wallet.StartTransaction()
	builder2 := wt.wallet.StartTransaction()
	// Fund each builder with a siacoin output that is smaller than all of the
	// outputs that the wallet should currently have.
	funding := types.NewCurrency64(10e3).Mul(types.SiacoinPrecision)
	err = builder1.FundSiacoins(funding)
	if err != nil {
		t.Fatal(err)
	}
	err = builder2.FundSiacoins(funding)
	if err != nil {
		t.Fatal(err)
	}

	// Get a second reading on the wallet's balance.
	fundedSCConfirmed, _, _ := wt.wallet.ConfirmedBalance()
	if !startingSCConfirmed.Equals(fundedSCConfirmed) {
		t.Fatal("confirmed siacoin balance changed when no blocks have been mined", startingSCConfirmed, fundedSCConfirmed)
	}

	// Spend the transaction funds on miner fees and the void output.
	builder1.AddMinerFee(types.NewCurrency64(25).Mul(types.SiacoinPrecision))
	builder2.AddMinerFee(types.NewCurrency64(25).Mul(types.SiacoinPrecision))
	// Send the money to the void.
	output := types.SiacoinOutput{Value: types.NewCurrency64(9975).Mul(types.SiacoinPrecision)}
	builder1.AddSiacoinOutput(output)
	builder2.AddSiacoinOutput(output)

	// Sign the transactions and verify that both are valid.
	tset1, err := builder1.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	tset2, err := builder2.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = wt.tpool.AcceptTransactionSet(tset1)
	if err != nil {
		t.Fatal(err)
	}
	err = wt.tpool.AcceptTransactionSet(tset2)
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block to get the transaction sets into the blockchain.
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
}

// TestConcurrentBuildersSingleOutput probes the behavior when multiple
// builders are created at the same time, but there is only a single wallet
// output that they end up needing to share.
func TestConcurrentBuildersSingleOutput(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Mine MaturityDelay blocks on the wallet using blocks that don't give
	// miner payouts to the wallet, so that all outputs can be condensed into a
	// single confirmed output. Currently the wallet will be getting a new
	// output per block because it has mined some blocks that haven't had their
	// outputs matured.
	for i := types.BlockHeight(0); i < types.MaturityDelay+1; i++ {
		err = wt.addBlockNoPayout()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Send all coins to a single confirmed output for the wallet.
	unlockConditions, err := wt.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	scBal, _, _ := wt.wallet.ConfirmedBalance()
	// Use a custom builder so that there is no transaction fee.
	builder := wt.wallet.StartTransaction()
	err = builder.FundSiacoins(scBal)
	if err != nil {
		t.Fatal(err)
	}
	output := types.SiacoinOutput{
		Value:      scBal,
		UnlockHash: unlockConditions.UnlockHash(),
	}
	builder.AddSiacoinOutput(output)
	tSet, err := builder.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = wt.tpool.AcceptTransactionSet(tSet)
	if err != nil {
		t.Fatal(err)
	}
	// Get the transaction into the blockchain without giving a miner payout to
	// the wallet.
	err = wt.addBlockNoPayout()
	if err != nil {
		t.Fatal(err)
	}

	// Get a baseline balance for the wallet.
	startingSCConfirmed, _, _ := wt.wallet.ConfirmedBalance()
	startingOutgoing, startingIncoming := wt.wallet.UnconfirmedBalance()
	if !startingOutgoing.IsZero() {
		t.Fatal(startingOutgoing)
	}
	if !startingIncoming.IsZero() {
		t.Fatal(startingIncoming)
	}

	// Create two builders at the same time, then add money to each.
	builder1 := wt.wallet.StartTransaction()
	builder2 := wt.wallet.StartTransaction()
	// Fund each builder with a siacoin output.
	funding := types.NewCurrency64(10e3).Mul(types.SiacoinPrecision)
	err = builder1.FundSiacoins(funding)
	if err != nil {
		t.Fatal(err)
	}
	// This add should fail, blocking the builder from completion.
	err = builder2.FundSiacoins(funding)
	if err != modules.ErrIncompleteTransactions {
		t.Fatal(err)
	}

	// Get a second reading on the wallet's balance.
	fundedSCConfirmed, _, _ := wt.wallet.ConfirmedBalance()
	if !startingSCConfirmed.Equals(fundedSCConfirmed) {
		t.Fatal("confirmed siacoin balance changed when no blocks have been mined", startingSCConfirmed, fundedSCConfirmed)
	}

	// Spend the transaction funds on miner fees and the void output.
	builder1.AddMinerFee(types.NewCurrency64(25).Mul(types.SiacoinPrecision))
	// Send the money to the void.
	output = types.SiacoinOutput{Value: types.NewCurrency64(9975).Mul(types.SiacoinPrecision)}
	builder1.AddSiacoinOutput(output)

	// Sign the transaction and submit it.
	tset1, err := builder1.Sign(true)
	if err != nil {
		t.Fatal(err)
	}
	err = wt.tpool.AcceptTransactionSet(tset1)
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block to get the transaction sets into the blockchain.
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
}

// TestParallelBuilders checks that multiple transaction builders can safely be
// opened at the same time, and that they will make valid transactions when
// building concurrently, using multiple gothreads to manage the builders.
func TestParallelBuilders(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// Mine a few more blocks so that the wallet has lots of outputs to pick
	// from.
	outputsDesired := 10
	for i := 0; i < outputsDesired; i++ {
		_, err := wt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Add MatruityDelay blocks with no payout to make tracking the balance
	// easier.
	for i := types.BlockHeight(0); i < types.MaturityDelay+1; i++ {
		err = wt.addBlockNoPayout()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Get a baseline balance for the wallet.
	startingSCConfirmed, _, _ := wt.wallet.ConfirmedBalance()
	startingOutgoing, startingIncoming := wt.wallet.UnconfirmedBalance()
	if !startingOutgoing.IsZero() {
		t.Fatal(startingOutgoing)
	}
	if !startingIncoming.IsZero() {
		t.Fatal(startingIncoming)
	}

	// Create several builders in parallel.
	var wg sync.WaitGroup
	funding := types.NewCurrency64(10e3).Mul(types.SiacoinPrecision)
	for i := 0; i < outputsDesired; i++ {
		wg.Add(1)
		go func() {
			// Create the builder and fund the transaction.
			builder := wt.wallet.StartTransaction()
			err := builder.FundSiacoins(funding)
			if err != nil {
				t.Fatal(err)
			}

			// Spend the transaction funds on miner fees and the void output.
			builder.AddMinerFee(types.NewCurrency64(25).Mul(types.SiacoinPrecision))
			output := types.SiacoinOutput{Value: types.NewCurrency64(9975).Mul(types.SiacoinPrecision)}
			builder.AddSiacoinOutput(output)
			// Sign the transactions and verify that both are valid.
			tset, err := builder.Sign(true)
			if err != nil {
				t.Fatal(err)
			}
			err = wt.tpool.AcceptTransactionSet(tset)
			if err != nil {
				t.Fatal(err)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	// Mine a block to get the transaction sets into the blockchain.
	err = wt.addBlockNoPayout()
	if err != nil {
		t.Fatal(err)
	}

	// Check the final balance.
	endingSCConfirmed, _, _ := wt.wallet.ConfirmedBalance()
	expected := startingSCConfirmed.Sub(funding.Mul(types.NewCurrency64(uint64(outputsDesired))))
	if !expected.Equals(endingSCConfirmed) {
		t.Fatal("did not get the expected ending balance", expected, endingSCConfirmed, startingSCConfirmed)
	}
}

func TestSignTransaction(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.closeWt()

	// get an output to spend
	outputs := wt.wallet.SpendableOutputs()

	// create a transaction that sends an output to the void
	txn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID: types.SiacoinOutputID(outputs[0].ID),
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      outputs[0].Value,
			UnlockHash: types.UnlockHash{},
		}},
	}

	// sign the transaction
	err = wt.wallet.SignTransaction(&txn, map[types.OutputID]types.UnlockHash{
		outputs[0].ID: outputs[0].RelatedAddress,
	})
	if err != nil {
		t.Fatal(err)
	}
	// txn should now have unlock condictions and a signature
	if txn.SiacoinInputs[0].UnlockConditions.SignaturesRequired == 0 {
		t.Fatal("unlock conditions are still unset")
	}
	if len(txn.TransactionSignatures) == 0 {
		t.Fatal("transaction was not signed")
	}

	// the resulting transaction should be valid; submit it to the tpool and
	// mine a block to confirm it
	err = txn.StandaloneValid(wt.wallet.Height())
	if err != nil {
		t.Fatal(err)
	}
	err = wt.tpool.AcceptTransactionSet([]types.Transaction{txn})
	if err != nil {
		t.Fatal(err)
	}
	wt.addBlockNoPayout()

	// the wallet should no longer list the resulting output as spendable
	outputs = wt.wallet.SpendableOutputs()
	if len(outputs) != 1 {
		t.Fatal("expected one output")
	}
	if outputs[0].ID == types.OutputID(txn.SiacoinInputs[0].ParentID) {
		t.Fatal("spent output still listed as spendable")
	}
}
