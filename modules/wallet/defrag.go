package wallet

import (
	"errors"
	"sort"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

var (
	errDefragNotNeeded = errors.New("defragging not needed, wallet is already sufficiently defragged")
)

// managedCreateDefragTransaction creates a transaction that spends multiple existing
// wallet outputs into a single new address.
func (w *Wallet) managedCreateDefragTransaction() ([]types.Transaction, error) {
	// dustThreshold and minFee have to be obtained separate from the lock
	dustThreshold, err := w.DustThreshold()
	if err != nil {
		return nil, err
	}
	minFee, _ := w.tpool.FeeEstimation()

	w.mu.Lock()
	defer w.mu.Unlock()

	consensusHeight, err := dbGetConsensusHeight(w.dbTx)
	if err != nil {
		return nil, err
	}

	// Collect a value-sorted set of siacoin outputs.
	var so sortedOutputs
	err = dbForEachSiacoinOutput(w.dbTx, func(scoid types.SiacoinOutputID, sco types.SiacoinOutput) {
		if w.checkOutput(w.dbTx, consensusHeight, scoid, sco, dustThreshold) == nil {
			so.ids = append(so.ids, scoid)
			so.outputs = append(so.outputs, sco)
		}
	})
	if err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(so))

	// Only defrag if there are enough outputs to merit defragging.
	if len(so.ids) <= defragThreshold {
		return nil, errDefragNotNeeded
	}

	// Skip over the 'defragStartIndex' largest outputs, so that the user can
	// still reasonably use their wallet while the defrag is happening.
	var amount types.Currency
	var parentTxn types.Transaction
	var spentScoids []types.SiacoinOutputID
	for i := defragStartIndex; i < defragStartIndex+defragBatchSize; i++ {
		scoid := so.ids[i]
		sco := so.outputs[i]

		// Add a siacoin input for this output.
		outputUnlockConditions := w.keys[sco.UnlockHash].UnlockConditions
		sci := types.SiacoinInput{
			ParentID:         scoid,
			UnlockConditions: outputUnlockConditions,
		}
		parentTxn.SiacoinInputs = append(parentTxn.SiacoinInputs, sci)
		spentScoids = append(spentScoids, scoid)

		// Add the output to the total fund
		amount = amount.Add(sco.Value)
	}

	// Create and add the output that will be used to fund the defrag
	// transaction.
	parentUnlockConditions, err := w.nextPrimarySeedAddress(w.dbTx)
	if err != nil {
		return nil, err
	}
	exactOutput := types.SiacoinOutput{
		Value:      amount,
		UnlockHash: parentUnlockConditions.UnlockHash(),
	}
	parentTxn.SiacoinOutputs = append(parentTxn.SiacoinOutputs, exactOutput)

	// Sign all of the inputs to the parent transaction.
	for _, sci := range parentTxn.SiacoinInputs {
		addSignatures(&parentTxn, types.FullCoveredFields, sci.UnlockConditions, crypto.Hash(sci.ParentID), w.keys[sci.UnlockConditions.UnlockHash()])
	}

	// Create the defrag transaction.
	refundAddr, err := w.nextPrimarySeedAddress(w.dbTx)
	if err != nil {
		return nil, err
	}

	// compute the transaction fee.
	sizeAvgOutput := uint64(250)
	fee := minFee.Mul64(sizeAvgOutput * defragBatchSize)

	txn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{{
			ParentID:         parentTxn.SiacoinOutputID(0),
			UnlockConditions: parentUnlockConditions,
		}},
		SiacoinOutputs: []types.SiacoinOutput{{
			Value:      amount.Sub(fee),
			UnlockHash: refundAddr.UnlockHash(),
		}},
		MinerFees: []types.Currency{fee},
	}
	addSignatures(&txn, types.FullCoveredFields, parentUnlockConditions, crypto.Hash(parentTxn.SiacoinOutputID(0)), w.keys[parentUnlockConditions.UnlockHash()])

	// Mark all outputs that were spent as spent.
	for _, scoid := range spentScoids {
		if err = dbPutSpentOutput(w.dbTx, types.OutputID(scoid), consensusHeight); err != nil {
			return nil, err
		}
	}
	// Mark the parent output as spent. Must be done after the transaction is
	// finished because otherwise the txid and output id will change.
	if err = dbPutSpentOutput(w.dbTx, types.OutputID(parentTxn.SiacoinOutputID(0)), consensusHeight); err != nil {
		return nil, err
	}

	// Construct the final transaction set
	return []types.Transaction{parentTxn, txn}, nil
}

// threadedDefragWallet computes the sum of the 15 largest outputs in the wallet and
// sends that sum to itself, effectively defragmenting the wallet. This defrag
// operation is only performed if the wallet has greater than defragThreshold
// outputs.
func (w *Wallet) threadedDefragWallet() {
	// Don't defrag if it was disabled
	if w.defragDisabled {
		return
	}

	err := w.tg.Add()
	if err != nil {
		return
	}
	defer w.tg.Done()

	// Check that a defrag makes sense.
	w.mu.RLock()
	unlocked := w.unlocked
	w.mu.RUnlock()
	if !unlocked {
		// Can't defrag if the wallet is locked.
		return
	}

	// Create the defrag transaction.
	txnSet, err := w.managedCreateDefragTransaction()
	defer func() {
		if err == nil {
			return
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		for _, txn := range txnSet {
			for _, sci := range txn.SiacoinInputs {
				dbDeleteSpentOutput(w.dbTx, types.OutputID(sci.ParentID))
			}
		}
	}()
	if err == errDefragNotNeeded {
		// begin
		return
	} else if err != nil {
		w.log.Println("WARN: couldn't create defrag transaction:", err)
		return
	}

	if w.deps.Disrupt("DefragInterrupted") {
		err = errors.New("defrag was interrupted (DefragInterrupted)")
		return
	}
	// Submit the defrag to the transaction pool.
	err = w.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		w.log.Println("WARN: defrag transaction was rejected:", err)
		return
	}
	w.log.Println("Submitting a transaction set to defragment the wallet's outputs, IDs:")
	for _, txn := range txnSet {
		w.log.Println("Wallet defrag: \t", txn.ID())
	}
}
