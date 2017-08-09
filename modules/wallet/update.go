package wallet

import (
	"fmt"
	"math"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// threadedResetSubscriptions unsubscribes the wallet from the consensus set and transaction pool
// and subscribes again.
func (w *Wallet) threadedResetSubscriptions() error {
	if !w.scanLock.TryLock() {
		return errScanInProgress
	}
	defer w.scanLock.Unlock()

	w.cs.Unsubscribe(w)
	w.tpool.Unsubscribe(w)

	err := w.cs.ConsensusSetSubscribe(w, modules.ConsensusChangeBeginning, w.tg.StopChan())
	if err != nil {
		return err
	}
	w.tpool.TransactionPoolSubscribe(w)
	return nil
}

// advanceSeedLookahead generates all keys from the current primary seed progress up to index
// and adds them to the set of spendable keys.  Therefore the new primary seed progress will
// be index+1 and new lookahead keys will be generated starting from index+1
// Returns true if a blockchain rescan is required
func (w *Wallet) advanceSeedLookahead(index uint64) (bool, error) {
	progress, err := dbGetPrimarySeedProgress(w.dbTx)
	if err != nil {
		return false, err
	}
	newProgress := index + 1

	// Add spendable keys and remove them from lookahead
	spendableKeys := generateKeys(w.primarySeed, progress, newProgress-progress)
	for _, key := range spendableKeys {
		w.keys[key.UnlockConditions.UnlockHash()] = key
		delete(w.lookahead, key.UnlockConditions.UnlockHash())
	}

	// Update the primarySeedProgress
	dbPutPrimarySeedProgress(w.dbTx, newProgress)
	if err != nil {
		return false, err
	}

	// Regenerate lookahead
	w.regenerateLookahead(newProgress)

	// If more than lookaheadRescanThreshold keys were generated
	// also initialize a rescan just to be safe.
	if uint64(len(spendableKeys)) > lookaheadRescanThreshold {
		return true, nil
	}

	return false, nil
}

// isWalletAddress is a helper function that checks if an UnlockHash is
// derived from one of the wallet's spendable keys or future keys.
func (w *Wallet) isWalletAddress(uh types.UnlockHash) bool {
	_, exists := w.keys[uh]
	return exists
}

// updateLookahead uses a consensus change to update the seed progress if one of the outputs
// contains an unlock hash of the lookahead set. Returns true if a blockchain rescan is required
func (w *Wallet) updateLookahead(tx *bolt.Tx, cc modules.ConsensusChange) (bool, error) {
	var largestIndex uint64
	for _, diff := range cc.SiacoinOutputDiffs {
		if index, ok := w.lookahead[diff.SiacoinOutput.UnlockHash]; ok {
			if index > largestIndex {
				largestIndex = index
			}
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		if index, ok := w.lookahead[diff.SiafundOutput.UnlockHash]; ok {
			if index > largestIndex {
				largestIndex = index
			}
		}
	}
	if largestIndex > 0 {
		return w.advanceSeedLookahead(largestIndex)
	}

	return false, nil
}

// updateConfirmedSet uses a consensus change to update the confirmed set of
// outputs as understood by the wallet.
func (w *Wallet) updateConfirmedSet(tx *bolt.Tx, cc modules.ConsensusChange) error {
	for _, diff := range cc.SiacoinOutputDiffs {
		// Verify that the diff is relevant to the wallet.
		if !w.isWalletAddress(diff.SiacoinOutput.UnlockHash) {
			continue
		}

		var err error
		if diff.Direction == modules.DiffApply {
			w.log.Println("Wallet has gained a spendable siacoin output:", diff.ID, "::", diff.SiacoinOutput.Value.HumanString())
			err = dbPutSiacoinOutput(tx, diff.ID, diff.SiacoinOutput)
		} else {
			w.log.Println("Wallet has lost a spendable siacoin output:", diff.ID, "::", diff.SiacoinOutput.Value.HumanString())
			err = dbDeleteSiacoinOutput(tx, diff.ID)
		}
		if err != nil {
			w.log.Severe("Could not update siacoin output:", err)
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		// Verify that the diff is relevant to the wallet.
		if !w.isWalletAddress(diff.SiafundOutput.UnlockHash) {
			continue
		}

		var err error
		if diff.Direction == modules.DiffApply {
			w.log.Println("Wallet has gained a spendable siafund output:", diff.ID, "::", diff.SiafundOutput.Value)
			err = dbPutSiafundOutput(tx, diff.ID, diff.SiafundOutput)
		} else {
			w.log.Println("Wallet has lost a spendable siafund output:", diff.ID, "::", diff.SiafundOutput.Value)
			err = dbDeleteSiafundOutput(tx, diff.ID)
		}
		if err != nil {
			w.log.Severe("Could not update siafund output:", err)
		}
	}
	for _, diff := range cc.SiafundPoolDiffs {
		var err error
		if diff.Direction == modules.DiffApply {
			err = dbPutSiafundPool(tx, diff.Adjusted)
		} else {
			err = dbPutSiafundPool(tx, diff.Previous)
		}
		if err != nil {
			w.log.Severe("Could not update siafund pool:", err)
		}
	}
	return nil
}

// revertHistory reverts any transaction history that was destroyed by reverted
// blocks in the consensus change.
func (w *Wallet) revertHistory(tx *bolt.Tx, reverted []types.Block) error {
	for _, block := range reverted {
		// Remove any transactions that have been reverted.
		for i := len(block.Transactions) - 1; i >= 0; i-- {
			// If the transaction is relevant to the wallet, it will be the
			// most recent transaction in bucketProcessedTransactions.
			txid := block.Transactions[i].ID()
			pt, err := dbGetLastProcessedTransaction(tx)
			if err != nil {
				break // bucket is empty
			}
			if txid == pt.TransactionID {
				w.log.Println("A wallet transaction has been reverted due to a reorg:", txid)
				if err := dbDeleteLastProcessedTransaction(tx); err != nil {
					w.log.Severe("Could not revert transaction:", err)
				}
			}
		}

		// Remove the miner payout transaction if applicable.
		for i, mp := range block.MinerPayouts {
			if w.isWalletAddress(mp.UnlockHash) {
				w.log.Println("Miner payout has been reverted due to a reorg:", block.MinerPayoutID(uint64(i)), "::", mp.Value.HumanString())
				if err := dbDeleteLastProcessedTransaction(tx); err != nil {
					w.log.Severe("Could not revert transaction:", err)
				}
				break // there will only ever be one miner transaction
			}
		}

		// decrement the consensus height
		if block.ID() != types.GenesisID {
			consensusHeight, err := dbGetConsensusHeight(tx)
			if err != nil {
				return err
			}
			err = dbPutConsensusHeight(tx, consensusHeight-1)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// applyHistory applies any transaction history that was introduced by the
// applied blocks.
func (w *Wallet) applyHistory(tx *bolt.Tx, cc modules.ConsensusChange) error {
	// compute spent outputs
	spentSiacoinOutputs := make(map[types.SiacoinOutputID]types.SiacoinOutput)
	spentSiafundOutputs := make(map[types.SiafundOutputID]types.SiafundOutput)
	for _, diff := range cc.SiacoinOutputDiffs {
		if diff.Direction == modules.DiffRevert {
			// revert means spent
			spentSiacoinOutputs[diff.ID] = diff.SiacoinOutput
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		if diff.Direction == modules.DiffRevert {
			// revert means spent
			spentSiafundOutputs[diff.ID] = diff.SiafundOutput
		}
	}

	for _, block := range cc.AppliedBlocks {
		consensusHeight, err := dbGetConsensusHeight(tx)
		if err != nil {
			return err
		}
		// increment the consensus height
		if block.ID() != types.GenesisID {
			consensusHeight++
			err = dbPutConsensusHeight(tx, consensusHeight)
			if err != nil {
				return err
			}
		}

		relevant := false
		for _, mp := range block.MinerPayouts {
			relevant = relevant || w.isWalletAddress(mp.UnlockHash)
		}
		if relevant {
			w.log.Println("Wallet has received new miner payouts:", block.ID())
			// Apply the miner payout transaction if applicable.
			minerPT := modules.ProcessedTransaction{
				Transaction:           types.Transaction{},
				TransactionID:         types.TransactionID(block.ID()),
				ConfirmationHeight:    consensusHeight,
				ConfirmationTimestamp: block.Timestamp,
			}
			for i, mp := range block.MinerPayouts {
				w.log.Println("\tminer payout:", block.MinerPayoutID(uint64(i)), "::", mp.Value.HumanString())
				minerPT.Outputs = append(minerPT.Outputs, modules.ProcessedOutput{
					ID:             types.OutputID(block.MinerPayoutID(uint64(i))),
					FundType:       types.SpecifierMinerPayout,
					MaturityHeight: consensusHeight + types.MaturityDelay,
					WalletAddress:  w.isWalletAddress(mp.UnlockHash),
					RelatedAddress: mp.UnlockHash,
					Value:          mp.Value,
				})
			}
			err := dbAppendProcessedTransaction(tx, minerPT)
			if err != nil {
				return fmt.Errorf("could not put processed miner transaction: %v", err)
			}
		}
		for _, txn := range block.Transactions {
			// determine if transaction is relevant
			relevant := false
			for _, sci := range txn.SiacoinInputs {
				relevant = relevant || w.isWalletAddress(sci.UnlockConditions.UnlockHash())
			}
			for _, sco := range txn.SiacoinOutputs {
				relevant = relevant || w.isWalletAddress(sco.UnlockHash)
			}
			for _, sfi := range txn.SiafundInputs {
				relevant = relevant || w.isWalletAddress(sfi.UnlockConditions.UnlockHash())
			}
			for _, sfo := range txn.SiafundOutputs {
				relevant = relevant || w.isWalletAddress(sfo.UnlockHash)
			}

			// only create a ProcessedTransaction if txn is relevant
			if !relevant {
				continue
			}
			w.log.Println("A transaction has been confirmed on the blockchain:", txn.ID())

			pt := modules.ProcessedTransaction{
				Transaction:           txn,
				TransactionID:         txn.ID(),
				ConfirmationHeight:    consensusHeight,
				ConfirmationTimestamp: block.Timestamp,
			}

			for _, sci := range txn.SiacoinInputs {
				pi := modules.ProcessedInput{
					ParentID:       types.OutputID(sci.ParentID),
					FundType:       types.SpecifierSiacoinInput,
					WalletAddress:  w.isWalletAddress(sci.UnlockConditions.UnlockHash()),
					RelatedAddress: sci.UnlockConditions.UnlockHash(),
					Value:          spentSiacoinOutputs[sci.ParentID].Value,
				}
				pt.Inputs = append(pt.Inputs, pi)

				// Log any wallet-relevant inputs.
				if pi.WalletAddress {
					w.log.Println("\tSiacoin Input:", pi.ParentID, "::", pi.Value.HumanString())
				}

			}

			for i, sco := range txn.SiacoinOutputs {
				po := modules.ProcessedOutput{
					ID:             types.OutputID(txn.SiacoinOutputID(uint64(i))),
					FundType:       types.SpecifierSiacoinOutput,
					MaturityHeight: consensusHeight,
					WalletAddress:  w.isWalletAddress(sco.UnlockHash),
					RelatedAddress: sco.UnlockHash,
					Value:          sco.Value,
				}
				pt.Outputs = append(pt.Outputs, po)

				// Log any wallet-relevant outputs.
				if po.WalletAddress {
					w.log.Println("\tSiacoin Output:", po.ID, "::", po.Value.HumanString())
				}
			}

			for _, sfi := range txn.SiafundInputs {
				pi := modules.ProcessedInput{
					ParentID:       types.OutputID(sfi.ParentID),
					FundType:       types.SpecifierSiafundInput,
					WalletAddress:  w.isWalletAddress(sfi.UnlockConditions.UnlockHash()),
					RelatedAddress: sfi.UnlockConditions.UnlockHash(),
					Value:          spentSiafundOutputs[sfi.ParentID].Value,
				}
				pt.Inputs = append(pt.Inputs, pi)
				// Log any wallet-relevant inputs.
				if pi.WalletAddress {
					w.log.Println("\tSiafund Input:", pi.ParentID, "::", pi.Value.HumanString())
				}

				siafundPool, err := dbGetSiafundPool(w.dbTx)
				if err != nil {
					return fmt.Errorf("could not get siafund pool: %v", err)
				}

				sfo := spentSiafundOutputs[sfi.ParentID]
				po := modules.ProcessedOutput{
					ID:             types.OutputID(sfi.ParentID),
					FundType:       types.SpecifierClaimOutput,
					MaturityHeight: consensusHeight + types.MaturityDelay,
					WalletAddress:  w.isWalletAddress(sfi.UnlockConditions.UnlockHash()),
					RelatedAddress: sfi.ClaimUnlockHash,
					Value:          siafundPool.Sub(sfo.ClaimStart).Mul(sfo.Value),
				}
				pt.Outputs = append(pt.Outputs, po)
				// Log any wallet-relevant outputs.
				if po.WalletAddress {
					w.log.Println("\tClaim Output:", po.ID, "::", po.Value.HumanString())
				}
			}

			for i, sfo := range txn.SiafundOutputs {
				po := modules.ProcessedOutput{
					ID:             types.OutputID(txn.SiafundOutputID(uint64(i))),
					FundType:       types.SpecifierSiafundOutput,
					MaturityHeight: consensusHeight,
					WalletAddress:  w.isWalletAddress(sfo.UnlockHash),
					RelatedAddress: sfo.UnlockHash,
					Value:          sfo.Value,
				}
				pt.Outputs = append(pt.Outputs, po)
				// Log any wallet-relevant outputs.
				if po.WalletAddress {
					w.log.Println("\tSiafund Output:", po.ID, "::", po.Value.HumanString())
				}
			}

			for _, fee := range txn.MinerFees {
				pt.Outputs = append(pt.Outputs, modules.ProcessedOutput{
					FundType: types.SpecifierMinerFee,
					Value:    fee,
				})
			}

			err := dbAppendProcessedTransaction(tx, pt)
			if err != nil {
				return fmt.Errorf("could not put processed transaction: %v", err)
			}
		}
	}

	return nil
}

// ProcessConsensusChange parses a consensus change to update the set of
// confirmed outputs known to the wallet.
func (w *Wallet) ProcessConsensusChange(cc modules.ConsensusChange) {
	if err := w.tg.Add(); err != nil {
		return
	}
	defer w.tg.Done()

	w.mu.Lock()
	defer w.mu.Unlock()

	if needRescan, err := w.updateLookahead(w.dbTx, cc); err != nil {
		w.log.Println("ERROR: failed to update lookahead:", err)
	} else if needRescan {
		go w.threadedResetSubscriptions()
	}
	if err := w.updateConfirmedSet(w.dbTx, cc); err != nil {
		w.log.Println("ERROR: failed to update confirmed set:", err)
	}
	if err := w.revertHistory(w.dbTx, cc.RevertedBlocks); err != nil {
		w.log.Println("ERROR: failed to revert consensus change:", err)
	}
	if err := w.applyHistory(w.dbTx, cc); err != nil {
		w.log.Println("ERROR: failed to apply consensus change:", err)
	}
	if err := dbPutConsensusChangeID(w.dbTx, cc.ID); err != nil {
		w.log.Println("ERROR: failed to update consensus change ID:", err)
	}

	if cc.Synced {
		go w.threadedDefragWallet()
	}
}

// ReceiveUpdatedUnconfirmedTransactions updates the wallet's unconfirmed
// transaction set.
func (w *Wallet) ReceiveUpdatedUnconfirmedTransactions(diff *modules.TransactionPoolDiff) {
	if err := w.tg.Add(); err != nil {
		return
	}
	defer w.tg.Done()

	w.mu.Lock()
	defer w.mu.Unlock()

	// Do the pruning first. If there are any pruned transactions, we will need
	// to re-allocate the whole processed transactions array.
	droppedTransactions := make(map[types.TransactionID]struct{})
	for i := range diff.RevertedTransactions {
		txids := w.unconfirmedSets[diff.RevertedTransactions[i]]
		for i := range txids {
			droppedTransactions[txids[i]] = struct{}{}
		}
		delete(w.unconfirmedSets, diff.RevertedTransactions[i])
	}

	// Skip the reallocation if we can, otherwise reallocate the
	// unconfirmedProcessedTransactions to no longer have the dropped
	// transactions.
	if len(droppedTransactions) != 0 {
		// Capacity can't be reduced, because we have no way of knowing if the
		// dropped transactions are relevant to the wallet or not, and some will
		// not be relevant to the wallet, meaning they don't have a counterpart
		// in w.unconfirmedProcessedTransactions.
		newUPT := make([]modules.ProcessedTransaction, 0, len(w.unconfirmedProcessedTransactions))
		for _, txn := range w.unconfirmedProcessedTransactions {
			_, exists := droppedTransactions[txn.TransactionID]
			if !exists {
				// Transaction was not dropped, add it to the new unconfirmed
				// transactions.
				newUPT = append(newUPT, txn)
			}
		}

		// Set the unconfirmed preocessed transactions to the pruned set.
		w.unconfirmedProcessedTransactions = newUPT
	}

	// Scroll through all of the diffs and add any new transactions.
	for _, unconfirmedTxnSet := range diff.AppliedTransactions {
		// Mark all of the transactions that appeared in this set.
		//
		// TODO: Technically only necessary to mark the ones that are relevant
		// to the wallet, but overhead should be low.
		w.unconfirmedSets[unconfirmedTxnSet.ID] = unconfirmedTxnSet.IDs

		// Get the values for the spent outputs.
		spentSiacoinOutputs := make(map[types.SiacoinOutputID]types.SiacoinOutput)
		for _, scod := range unconfirmedTxnSet.Change.SiacoinOutputDiffs {
			// Only need to grab the reverted ones, because only reverted ones
			// have the possibility of having been spent.
			if scod.Direction == modules.DiffRevert {
				spentSiacoinOutputs[scod.ID] = scod.SiacoinOutput
			}
		}

		// Add each transaction to our set of unconfirmed transactions.
		for i, txn := range unconfirmedTxnSet.Transactions {
			// determine whether transaction is relevant to the wallet
			relevant := false
			for _, sci := range txn.SiacoinInputs {
				relevant = relevant || w.isWalletAddress(sci.UnlockConditions.UnlockHash())
			}
			for _, sco := range txn.SiacoinOutputs {
				relevant = relevant || w.isWalletAddress(sco.UnlockHash)
			}

			// only create a ProcessedTransaction if txn is relevant
			if !relevant {
				continue
			}

			pt := modules.ProcessedTransaction{
				Transaction:           txn,
				TransactionID:         unconfirmedTxnSet.IDs[i],
				ConfirmationHeight:    types.BlockHeight(math.MaxUint64),
				ConfirmationTimestamp: types.Timestamp(math.MaxUint64),
			}
			for _, sci := range txn.SiacoinInputs {
				pt.Inputs = append(pt.Inputs, modules.ProcessedInput{
					ParentID:       types.OutputID(sci.ParentID),
					FundType:       types.SpecifierSiacoinInput,
					WalletAddress:  w.isWalletAddress(sci.UnlockConditions.UnlockHash()),
					RelatedAddress: sci.UnlockConditions.UnlockHash(),
					Value:          spentSiacoinOutputs[sci.ParentID].Value,
				})
			}
			for i, sco := range txn.SiacoinOutputs {
				pt.Outputs = append(pt.Outputs, modules.ProcessedOutput{
					ID:             types.OutputID(txn.SiacoinOutputID(uint64(i))),
					FundType:       types.SpecifierSiacoinOutput,
					MaturityHeight: types.BlockHeight(math.MaxUint64),
					WalletAddress:  w.isWalletAddress(sco.UnlockHash),
					RelatedAddress: sco.UnlockHash,
					Value:          sco.Value,
				})
			}
			for _, fee := range txn.MinerFees {
				pt.Outputs = append(pt.Outputs, modules.ProcessedOutput{
					FundType: types.SpecifierMinerFee,
					Value:    fee,
				})
			}
			w.unconfirmedProcessedTransactions = append(w.unconfirmedProcessedTransactions, pt)
		}
	}
}
