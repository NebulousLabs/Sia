package wallet

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// revertOutput reverts an output from the wallet. The output must be the
// output in the most recent wallet transaction of the wallet transaction array
// if the unlock hash belongs to the wallet. If the unlock hash is
// unrecognized, this function is a no-op.
func (w *Wallet) revertWalletTransaction(uh UnlockHash, wtid modules.WalletTransactionID) {
	_, exists := w.generatedKeys(uh)
	if exists {
		// Sanity check - the output should exist in the wallet transaction map
		// because a prior addition to the map is being reverted.
		_, exists := w.walletTransactionMap[wtid]
		if !exists && build.DEBUG {
			panic("wallet transaction not found in the wallet transaction map")
		}
		delete(w.walletTransactionMap, wtid)

		// Sanity check - the last element of the wallet transaction array
		// should be the item we are deleteing.
		lastIndex := len(w.walletTransactions)-1
		if wtid != walletTransactionID(w.walletTransactions[lastIndex].TransactionID, w.walletTransactions[lastIndex].OutputID) {
			panic("wallet transactions are being deleted in the wrong order")
		}
		w.walletTransactions = w.walletTransactions[:lastIndex]
		return true
	}
	return false
}

// applyWalletTransaction adds a wallet transaction to the wallet transaction
// history.
func (w *Wallet) applyWalletTransaction(fundType types.Specifier, uh UnlockHash, t types.Transaction, confirmationTime types.Timestamp, oid types.OutputID, value types.Currency) {
	_, exists := w.generatedKeys(uh)
	if exists {
		// Sanity check - the output should not exist in the wallet transaction
		// map, this should be the first time it was created.
		wtid := modules.CalculateWalletTransactionID(t.ID(), oid)
		_, exists := w.walletTransactionMap[wtid]
		if exists && build.DEBUG {
			panic("a wallet transaction is being added for the second time")
		}
		wt := WalletTransaction{
			WalletTransactionID: wtid,
			ConfirmationHeight: w.consensusSetHeight,
			ConfirmationTimestamp: confirmationTime,
			Transaction: t,

			FundType: fundType,
			OutputID: oid,
			RelatedAddress: uh,
			Value: value,
		}
		w.walletTransactionMap[wtid] = wt
		w.walletTransactions = append(w.walletTransactions, wt)
		w.historicOutputs[oid] = value
		return true
	}
	return false
}

// ProcessConsensusChange parses a consensus change to update the set of
// confirmed outputs known to the wallet.
func (w *Wallet) ProcessConsensusChange(cc modules.ConsensusChange) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	// Iterate through the output diffs (siacoin and siafund) and apply all of
	// them. Only apply the outputs that relate to unlock hashes we understand.
	for _, diff := range cc.SiacoinOutputDiffs {
		_, exists := w.SiacoinOutputs[diff.ID]
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
		_, exists := w.SiafundOutputs[diff.ID]
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
	for _, block := cc.RevertedBlocks {
		for _, txn := range block.Transactions {
			// Revert all wallet transactions made from items in the
			// transaction - use the reverse order of apply because using a
			// slice means its easiet to remove the last element.
			txid := txn.ID()
			for i := len(txn.SiafundOutputs)-1; i >= 0; i-- {
				w.revertWalletTransaction(txn.SiafundOutputs[i].UnlockHash, walletTransactionID(txid, OutputID(txn.SiafundOutputID(i))))
			}
			for i := len(txn.SiafundInputs)-1; i >= 0; i-- {
				w.revertWalletTransaction(txn.SiafundInputs[i].ClaimUnlockHash, walletTransactionID(txid, OutputID(txn.SiaClaimOutputID(i))))
				w.revertWalletTransaction(txn.SiafundInputs[i].UnlockHash, walletTransactionID(txid, OutputID(txn.SiafundInputs[i].ParentID)))
			}
			for i := len(txn.SiacoinOutputs)-1; i >= 0; i-- {
				w.revertWalletTransaction(txn.SiacoinOutputs[i].UnlockHash, walletTransactionID(txid, OutputID(txn.SiacoinOutputID(i))))
			}
			for i := len(txn.SiacoinInputs)-1; i >= 0; i-- {
				w.revertWalletTransaction(txn.SiacoinInputs[i].UnlockHash, walletTransactionID(txid, OutputID(txn.SiacoinInputs[i].ParentID)))
			}

		}
		for i := len(block.MinerPayouts)-1; i >= 0; i-- {
			w.revertWalletTransaction(block.MinerPayouts[i].UnlockHash, OutputID(block.MinerPayoutID(i)))
		}
	}

	// Apply all of the new blocks.
	for _, block := range cc.AppliedBlocks {
		// Apply any miner outputs.
		for i, mp := range block.MinerPayouts {
			w.applyWalletTransaction(mp.UnlockHash, types.Transaction{}, block.Timestamp, OutputID(block.MinerPayoutID(i)), mp.Value)
		}
		for _, txn := range block.Transactions {
			// Add a wallet transaction for all transaction elements.
			for _, sci := range txn.SiacoinInputs{
				w.applyWalletTransaction(types.SpecifierSiacoinInput, sci.UnlockConditions.UnlockHash(), txn, block.Timestamp, OutputID(sci.ParentID), w.historicOutputs[OutputID(sci.ParentID)])
			}
			for i, sco := range txn.SiacoinOutputs {
				w.applyWalletTransaction(types.SpecifierSiacoinOutput, sco.UnlockHash, txn, block.Timestamp, OutputID(txn.SiacoinOutputID(i)), sco.Value)
			}
			for _, sfi := range txn.SiafundInputs{
				w.applyWalletTransaction(types.SpecifierSiafundInput, sfi.UnlockConditions.UnlockHash(), txn, block.Timestamp, OutputID(sfi.ParentID), w.historicOutputs[OutputID(sfi.ParentID)])
			}
			for i, sfo := range txn.SiafundOutputs {
				w.applyWalletTransaction(types.SpecifierSiafundOutput, sfo.UnlockHash, txn, block.Timestamp, OutputID(txn.SiafundOutputID(i)), sfo.Value)
			}
		}
	}
}

// ReceiveUpdatedUnconfirmedTransactions updates the wallet's unconfirmed
// transaction set.
func (w *Wallet) ReceiveUpdatedUnconfirmedTransactions(txns []types.Transaction, _ modules.ConsensusChange) {
	lockID := w.mu.Lock()
	defer w.mu.Unlock(lockID)

	w.unconfirmedWalletTransactions = nil
	for _, txn :=range txns {
		for _, sci := range txn.SiacoinInputs {
			_, exists := w.generatedKeys(sci.UnlockConditions.UnlockHash())
			if exists {
				wt := WalletTransaction{
					WalletTransactionID: modules.WalletTransactionID(txn.ID(), sci.UnlockConditions.UnlockHash()),
					ConfirmationHeight: types.BlockHeight(0) - 1,
					ConfirmationTimestamp: types.Timestamp(0) - 1,
					Transaction: txn,

					FundType: types.SpecifierSiacoinInput,
					OutputID: OutputID(sci.ParentID),
					RelatedAddress: sci.UnlockConditions.UnlockHash(),
					Value: w.historicalOutputs[oid],
				}
				w.unconfirmedWalletTransactions = append(w.walletTransactions, wt)
			}
		}
		for i, sco := range txn.SiacoinOutputs {
			_, exists := w.generatedKeys(sco.UnlockHash)
			if exists {
				wt := WalletTransaction{
					WalletTransactionID: modules.WalletTransactionID(txn.ID(), sco.UnlockHash),
					ConfirmationHeight: types.BlockHeight(0) - 1,
					ConfirmationTimestamp: types.Timestamp(0) - 1,
					Transaction: txn,

					FundType: types.SpecifierSiacoinOutput,
					OutputID: OutputID(txn.SiacoinOutputID(i)),
					RelatedAddress: sco.UnlockHash,
					Value: sco.Value,
				}
				w.unconfirmedWalletTransactions = append(w.walletTransactions, wt)
				w.historicOutputs[oid] = value
			}
		}
	}
}
