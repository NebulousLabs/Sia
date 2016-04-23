package wallet

import (
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// TestViewAdded checks that 'ViewAdded' returns sane-seeming values when
// indicating which elements have been added automatically to a transaction
// set.
func TestViewAdded(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := createWalletTester("TestViewAdded")
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
	wt, err := createWalletTester("TestDoubleSignError")
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
