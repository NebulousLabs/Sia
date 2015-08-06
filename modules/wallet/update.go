package wallet

import (
	"math"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// revertOutput reverts an output from the wallet. The output must be the
// output in the most recent wallet transaction of the wallet transaction array
// if the unlock hash belongs to the wallet. If the unlock hash is
// unrecognized, this function is a no-op.
func (w *Wallet) revertWalletTransaction(uh types.UnlockHash, wtid modules.WalletTransactionID) {
	_, exists := w.keys[uh]
	if exists {
		// Sanity check - the output should exist in the wallet transaction map
		// because a prior addition to the map is being reverted.
		_, exists := w.walletTransactionMap[wtid]
		if build.DEBUG && !exists {
			panic("wallet transaction not found in the wallet transaction map")
		}
		delete(w.walletTransactionMap, wtid)

		// Sanity check - the last element of the wallet transaction array
		// should be the item we are deleteing.
		lastIndex := len(w.walletTransactions) - 1
		if build.DEBUG && wtid != modules.CalculateWalletTransactionID(w.walletTransactions[lastIndex].TransactionID, w.walletTransactions[lastIndex].OutputID) {
			panic("wallet transactions are being deleted in the wrong order")
		}
		w.walletTransactions = w.walletTransactions[:lastIndex]
		return
	}
	return
}

// applyWalletTransaction adds a wallet transaction to the wallet transaction
// history.
func (w *Wallet) applyWalletTransaction(fundType types.Specifier, uh types.UnlockHash, t types.Transaction, confirmationTime types.Timestamp, oid types.OutputID, value types.Currency) {
	_, exists := w.keys[uh]
	if exists {
		// Sanity check - the output should not exist in the wallet transaction
		// map, this should be the first time it was created.
		wtid := modules.CalculateWalletTransactionID(t.ID(), oid)
		_, exists := w.walletTransactionMap[wtid]
		if exists && build.DEBUG {
			panic("a wallet transaction is being added for the second time")
		}
		wt := modules.WalletTransaction{
			TransactionID:         t.ID(),
			ConfirmationHeight:    w.consensusSetHeight,
			ConfirmationTimestamp: confirmationTime,
			Transaction:           t,

			FundType:       fundType,
			OutputID:       oid,
			RelatedAddress: uh,
			Value:          value,
		}
		w.walletTransactions = append(w.walletTransactions, wt)
		w.walletTransactionMap[wtid] = &w.walletTransactions[len(w.walletTransactions)-1]
		w.historicOutputs[oid] = value
		return
	}
	return
}

// ProcessConsensusChange parses a consensus change to update the set of
// confirmed outputs known to the wallet.
func (w *Wallet) ProcessConsensusChange(cc modules.ConsensusChange) {
	// There are two different situations under which a subscribee calls
	// ProcessConsensusChange. The first is when w.subscribed is set to false
	// AND the mutex is already locked. The other situation is that subscribed
	// is set to true and is not going to be changed. Therefore there is no
	// race condition here. If w.subscribed is set to false, trying to grab the
	// lock would cause a deadlock.
	if w.subscribed {
		lockID := w.mu.Lock()
		defer w.mu.Unlock(lockID)
	}

	// Iterate through the output diffs (siacoin and siafund) and apply all of
	// them. Only apply the outputs that relate to unlock hashes we understand.
	for _, diff := range cc.SiacoinOutputDiffs {
		// Verify that the diff is relevant to the wallet.
		_, exists := w.keys[diff.SiacoinOutput.UnlockHash]
		if !exists {
			continue
		}

		_, exists = w.siacoinOutputs[diff.ID]
		if diff.Direction == modules.DiffApply {
			if exists && build.DEBUG {
				panic("adding an existing output to wallet")
			}
			w.siacoinOutputs[diff.ID] = diff.SiacoinOutput
		} else {
			if !exists && build.DEBUG {
				panic("deleting nonexisting output from wallet")
			}
			delete(w.siacoinOutputs, diff.ID)
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		// Verify that the diff is relevant to the wallet.
		_, exists := w.keys[diff.SiafundOutput.UnlockHash]
		if !exists {
			continue
		}

		_, exists = w.siafundOutputs[diff.ID]
		if diff.Direction == modules.DiffApply {
			if exists && build.DEBUG {
				panic("adding an existing output to wallet")
			}
			w.siafundOutputs[diff.ID] = diff.SiafundOutput
		} else {
			if !exists && build.DEBUG {
				panic("deleting nonexisting output from wallet")
			}
			delete(w.siafundOutputs, diff.ID)
		}
	}
	for _, diff := range cc.SiafundPoolDiffs {
		if diff.Direction == modules.DiffApply {
			w.siafundPool = diff.Adjusted
		} else {
			w.siafundPool = diff.Previous
		}
	}

	// Iterate through the transactions and find every transaction somehow
	// related to the wallet. Wallet transactions must be removed in the same
	// order they were added.
	for _, block := range cc.RevertedBlocks {
		for _, txn := range block.Transactions {
			// Revert all wallet transactions made from items in the
			// transaction - use the reverse order of apply because using a
			// slice means its easiet to remove the last element.
			txid := txn.ID()
			for i := len(txn.SiafundOutputs) - 1; i >= 0; i-- {
				w.revertWalletTransaction(txn.SiafundOutputs[i].UnlockHash, modules.CalculateWalletTransactionID(txid, types.OutputID(txn.SiafundOutputID(i))))
			}
			for i := len(txn.SiafundInputs) - 1; i >= 0; i-- {
				sfoid := txn.SiafundOutputID(i)
				w.revertWalletTransaction(txn.SiafundInputs[i].ClaimUnlockHash, modules.CalculateWalletTransactionID(txid, types.OutputID(sfoid.SiaClaimOutputID())))
				w.revertWalletTransaction(txn.SiafundInputs[i].UnlockConditions.UnlockHash(), modules.CalculateWalletTransactionID(txid, types.OutputID(txn.SiafundInputs[i].ParentID)))
			}
			for i := len(txn.SiacoinOutputs) - 1; i >= 0; i-- {
				w.revertWalletTransaction(txn.SiacoinOutputs[i].UnlockHash, modules.CalculateWalletTransactionID(txid, types.OutputID(txn.SiacoinOutputID(i))))
			}
			for i := len(txn.SiacoinInputs) - 1; i >= 0; i-- {
				w.revertWalletTransaction(txn.SiacoinInputs[i].UnlockConditions.UnlockHash(), modules.CalculateWalletTransactionID(txid, types.OutputID(txn.SiacoinInputs[i].ParentID)))
			}

		}
		for i := len(block.MinerPayouts) - 1; i >= 0; i-- {
			w.revertWalletTransaction(block.MinerPayouts[i].UnlockHash, modules.CalculateWalletTransactionID(types.Transaction{}.ID(), types.OutputID(block.MinerPayoutID(uint64(i)))))
		}
	}

	// Apply all of the new blocks.
	for _, block := range cc.AppliedBlocks {
		// Apply any miner outputs.
		for i, mp := range block.MinerPayouts {
			w.applyWalletTransaction(types.SpecifierMinerPayout, mp.UnlockHash, types.Transaction{}, block.Timestamp, types.OutputID(block.MinerPayoutID(uint64(i))), mp.Value)
		}
		for _, txn := range block.Transactions {
			// Add a wallet transaction for all transaction elements.
			for _, sci := range txn.SiacoinInputs {
				w.applyWalletTransaction(types.SpecifierSiacoinInput, sci.UnlockConditions.UnlockHash(), txn, block.Timestamp, types.OutputID(sci.ParentID), w.historicOutputs[types.OutputID(sci.ParentID)])
			}
			for i, sco := range txn.SiacoinOutputs {
				w.applyWalletTransaction(types.SpecifierSiacoinOutput, sco.UnlockHash, txn, block.Timestamp, types.OutputID(txn.SiacoinOutputID(i)), sco.Value)
			}
			for _, sfi := range txn.SiafundInputs {
				w.applyWalletTransaction(types.SpecifierSiafundInput, sfi.UnlockConditions.UnlockHash(), txn, block.Timestamp, types.OutputID(sfi.ParentID), w.historicOutputs[types.OutputID(sfi.ParentID)])
			}
			for i, sfo := range txn.SiafundOutputs {
				w.applyWalletTransaction(types.SpecifierSiafundOutput, sfo.UnlockHash, txn, block.Timestamp, types.OutputID(txn.SiafundOutputID(i)), sfo.Value)
			}
		}
	}
}

// ReceiveUpdatedUnconfirmedTransactions updates the wallet's unconfirmed
// transaction set.
func (w *Wallet) ReceiveUpdatedUnconfirmedTransactions(txns []types.Transaction, _ modules.ConsensusChange) {
	// There are two different situations under which a subscribee calls
	// ProcessConsensusChange. The first is when w.subscribed is set to false
	// AND the mutex is already locked. The other situation is that subscribed
	// is set to true and is not going to be changed. Therefore there is no
	// race condition here. If w.subscribed is set to false, trying to grab the
	// lock would cause a deadlock.
	if w.subscribed {
		lockID := w.mu.Lock()
		defer w.mu.Unlock(lockID)
	}

	w.unconfirmedWalletTransactions = nil
	for _, txn := range txns {
		for _, sci := range txn.SiacoinInputs {
			_, exists := w.keys[sci.UnlockConditions.UnlockHash()]
			if exists {
				wt := modules.WalletTransaction{
					TransactionID:         txn.ID(),
					ConfirmationHeight:    types.BlockHeight(math.MaxUint64),
					ConfirmationTimestamp: types.Timestamp(math.MaxUint64),
					Transaction:           txn,

					FundType:       types.SpecifierSiacoinInput,
					OutputID:       types.OutputID(sci.ParentID),
					RelatedAddress: sci.UnlockConditions.UnlockHash(),
					Value:          w.historicOutputs[types.OutputID(sci.ParentID)],
				}
				w.unconfirmedWalletTransactions = append(w.unconfirmedWalletTransactions, wt)
			}
		}
		for i, sco := range txn.SiacoinOutputs {
			_, exists := w.keys[sco.UnlockHash]
			if exists {
				wt := modules.WalletTransaction{
					TransactionID:         txn.ID(),
					ConfirmationHeight:    types.BlockHeight(math.MaxUint64),
					ConfirmationTimestamp: types.Timestamp(math.MaxUint64),
					Transaction:           txn,

					FundType:       types.SpecifierSiacoinOutput,
					OutputID:       types.OutputID(txn.SiacoinOutputID(i)),
					RelatedAddress: sco.UnlockHash,
					Value:          sco.Value,
				}
				w.unconfirmedWalletTransactions = append(w.unconfirmedWalletTransactions, wt)
				oid := types.OutputID(txn.SiacoinOutputID(i))
				w.historicOutputs[oid] = sco.Value
			}
		}
	}
}
