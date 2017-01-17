package wallet

import (
	"errors"
	"sort"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errDefragNotNeeded = errors.New("defragging not needed, wallet is already sufficiently defragged")
)

// fundDefragger is a private function which funds the defragger transaction.
// This helper func is needed because the lock on the wallet cannot be dropped
// throughout scanning the outputs to determine if defragmentation is necessary
// and then proceeding to actually defrag.
func (tb *transactionBuilder) fundDefragger(fee types.Currency) (types.Currency, error) {
	// Sanity check
	if build.DEBUG && defragThreshold <= defragBatchSize+defragStartIndex {
		panic("constants are incorrect, defragThreshold needs to be larger than the sum of defragBatchSize and defragStartIndex")
	}

	tb.wallet.mu.Lock()
	defer tb.wallet.mu.Unlock()

	// Only defrag if the wallet is unlocked.
	if !tb.wallet.unlocked {
		return types.Currency{}, errDefragNotNeeded
	}

	// Collect a set of outputs for defragging.
	var so sortedOutputs
	var num int
	for scoid, sco := range tb.wallet.siacoinOutputs {
		// Skip over any outputs that aren't actually spendable.
		if err := tb.wallet.checkOutput(scoid, sco); err != nil {
			continue
		}
		so.ids = append(so.ids, scoid)
		so.outputs = append(so.outputs, sco)
		num++
	}

	// Only defrag if there are enough outputs to merit defragging.
	if num <= defragThreshold {
		return types.Currency{}, errDefragNotNeeded
	}

	// Sort the outputs by size.
	sort.Sort(sort.Reverse(so))

	// Use all of the smaller outputs to fund the transaction, tracking the
	// total number of coins used to fund the transaction.
	var amount types.Currency
	parentTxn := types.Transaction{}
	var spentScoids []types.SiacoinOutputID
	for i := defragStartIndex; i < defragStartIndex+defragBatchSize; i++ {
		scoid := so.ids[i]
		sco := so.outputs[i]

		// Add a siacoin input for this output.
		outputUnlockConditions := tb.wallet.keys[sco.UnlockHash].UnlockConditions
		sci := types.SiacoinInput{
			ParentID:         scoid,
			UnlockConditions: outputUnlockConditions,
		}
		parentTxn.SiacoinInputs = append(parentTxn.SiacoinInputs, sci)
		spentScoids = append(spentScoids, scoid)

		// Add the output to the total fund
		amount = amount.Add(sco.Value)
	}

	// Create and add the output that will be used to fund the standard
	// transaction.
	parentUnlockConditions, err := tb.wallet.nextPrimarySeedAddress()
	if err != nil {
		return types.Currency{}, err
	}
	exactOutput := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: parentUnlockConditions.UnlockHash(),
	}
	parentTxn.SiacoinOutputs = append(parentTxn.SiacoinOutputs, exactOutput)

	// Sign all of the inputs to the parent trancstion.
	for _, sci := range parentTxn.SiacoinInputs {
		_, err := addSignatures(&parentTxn, types.FullCoveredFields, sci.UnlockConditions, crypto.Hash(sci.ParentID), tb.wallet.keys[sci.UnlockConditions.UnlockHash()])
		if err != nil {
			return types.Currency{}, err
		}
	}
	// Mark the parent output as spent. Must be done after the transaction is
	// finished because otherwise the txid and output id will change.
	tb.wallet.spentOutputs[types.OutputID(parentTxn.SiacoinOutputID(0))] = tb.wallet.consensusSetHeight

	// Add the exact output.
	newInput := types.SiacoinInput{
		ParentID:         parentTxn.SiacoinOutputID(0),
		UnlockConditions: parentUnlockConditions,
	}
	tb.newParents = append(tb.newParents, len(tb.parents))
	tb.parents = append(tb.parents, parentTxn)
	tb.siacoinInputs = append(tb.siacoinInputs, len(tb.transaction.SiacoinInputs))
	tb.transaction.SiacoinInputs = append(tb.transaction.SiacoinInputs, newInput)

	// Mark all outputs that were spent as spent.
	for _, scoid := range spentScoids {
		tb.wallet.spentOutputs[types.OutputID(scoid)] = tb.wallet.consensusSetHeight
	}
	return amount, nil
}

// threadedDefragWallet computes the sum of the 15 largest outputs in the wallet and
// sends that sum to itself, effectively defragmenting the wallet. This defrag
// operation is only performed if the wallet has greater than defragThreshold
// outputs.
func (w *Wallet) threadedDefragWallet() {
	err := w.tg.Add()
	if err != nil {
		return
	}
	defer w.tg.Done()

	// grab a new address from the wallet
	w.mu.Lock()
	addr, err := w.nextPrimarySeedAddress()
	w.mu.Unlock()
	if err != nil {
		w.log.Println("Error getting an address for defragmentation: ", err)
		return
	}

	// Create a transaction builder.
	fee := defragFee()
	tbuilder := w.registerTransaction(types.Transaction{}, nil)
	// Fund it using a defragging specific method.
	amount, err := tbuilder.fundDefragger(fee)
	if err != nil {
		if err != errDefragNotNeeded {
			w.log.Println("Error while trying to fund the defragging transaction", err)
		}
		return
	}
	// Add the miner fee.
	tbuilder.AddMinerFee(fee)
	// Add the refund.
	tbuilder.AddSiacoinOutput(types.SiacoinOutput{
		Value:      amount.Sub(fee),
		UnlockHash: addr.UnlockHash(),
	})
	// Sign the transaction.
	txns, err := tbuilder.Sign(true)
	if err != nil {
		w.log.Println("Error signing transaction set in defrag transaction: ", err)
		return
	}
	// Submit the defrag to the transaction pool.
	err = w.tpool.AcceptTransactionSet(txns)
	if err != nil {
		w.log.Println("Error accepting transaction set in defrag transaction: ", err)
	}
}
