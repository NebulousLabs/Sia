package wallet

import (
	"errors"
	"sort"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
)

var (
	errDefragNotNeeded = errors.New("defragging not needed, wallet is already sufficiently defragged")
)

// createDefragTransaction creates a transaction that spends multiple existing
// wallet outputs into a single new address.
func (w *Wallet) createDefragTransaction() (txnSet []types.Transaction, err error) {
	err = w.db.Update(func(tx *bolt.Tx) error {
		consensusHeight, err := dbGetConsensusHeight(tx)
		if err != nil {
			return err
		}

		// Collect a value-sorted set of siacoin outputs.
		var so sortedOutputs
		err = dbForEachSiacoinOutput(tx, func(scoid types.SiacoinOutputID, sco types.SiacoinOutput) {
			if w.checkOutput(tx, consensusHeight, scoid, sco) == nil {
				so.ids = append(so.ids, scoid)
				so.outputs = append(so.outputs, sco)
			}
		})
		if err != nil {
			return err
		}
		sort.Sort(sort.Reverse(so))

		// Only defrag if there are enough outputs to merit defragging.
		if len(so.ids) <= defragThreshold {
			return errDefragNotNeeded
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
		parentUnlockConditions, err := w.nextPrimarySeedAddress(tx)
		if err != nil {
			return err
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
		fee := defragFee()
		refundAddr, err := w.nextPrimarySeedAddress(tx)
		if err != nil {
			return err
		}
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
			if err := dbPutSpentOutput(tx, types.OutputID(scoid), consensusHeight); err != nil {
				return err
			}
		}
		// Mark the parent output as spent. Must be done after the transaction is
		// finished because otherwise the txid and output id will change.
		if err := dbPutSpentOutput(tx, types.OutputID(parentTxn.SiacoinOutputID(0)), consensusHeight); err != nil {
			return err
		}

		// Construct the final transaction set
		txnSet = []types.Transaction{parentTxn, txn}
		return nil
	})
	return
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

	// Check that a defrag makes sense.
	w.mu.RLock()
	// Can't defrag if the wallet is locked.
	if !w.unlocked {
		w.mu.RUnlock()
		return
	}
	// No need to defrag if the number of outputs is below the defrag limit.
	// NOTE: some outputs may be invalid (e.g. dust), but it's still more
	// efficient to count them naively as a first pass.
	var totalOutputs int
	w.db.View(func(tx *bolt.Tx) error {
		totalOutputs = tx.Bucket(bucketSiacoinOutputs).Stats().KeyN
		return nil
	})
	w.mu.RUnlock()
	if totalOutputs < defragThreshold {
		return
	}

	// Create the defrag transaction.
	w.mu.Lock()
	txnSet, err := w.createDefragTransaction()
	w.mu.Unlock()
	if err == errDefragNotNeeded {
		// benign
		return
	} else if err != nil {
		w.log.Println("WARN: couldn't create defrag transaction:", err)
		return
	}
	// Submit the defrag to the transaction pool.
	err = w.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		w.log.Println("WARN: defrag transaction was rejected:", err)
	}
}
