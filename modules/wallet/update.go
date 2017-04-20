package wallet

import (
	"bytes"
	"fmt"
	"math"
	"sort"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// historicOutput defines a historic output as recognized by the wallet. This
// struct is primarily used to sort the historic outputs before inserting them
// into the bolt database.
type historicOutput struct {
	id  types.OutputID
	val types.Currency
}

// isWalletAddress is a helper function that checks if an UnlockHash is
// derived from one of the wallet's spendable keys.
func (w *Wallet) isWalletAddress(uh types.UnlockHash) bool {
	_, exists := w.keys[uh]
	return exists
}

// updateConfirmedSet uses a consensus change to update the confirmed set of
// outputs as understood by the wallet.
func (w *Wallet) updateConfirmedSet(tx *bolt.Tx, cc modules.ConsensusChange) error {
	var historicOutputs []historicOutput
	for _, diff := range cc.SiacoinOutputDiffs {
		// Add to historic outputs.
		// NOTE: it's never necessary to delete from the historic output set.
		if diff.Direction == modules.DiffApply {
			historicOutputs = append(historicOutputs, historicOutput{types.OutputID(diff.ID), diff.SiacoinOutput.Value})
		}
		// Verify that the diff is relevant to the wallet.
		if !w.isWalletAddress(diff.SiacoinOutput.UnlockHash) {
			continue
		}

		var err error
		if diff.Direction == modules.DiffApply {
			err = dbPutSiacoinOutput(tx, diff.ID, diff.SiacoinOutput)
		} else {
			err = dbDeleteSiacoinOutput(tx, diff.ID)
		}
		if err != nil {
			w.log.Severe("Could not update siacoin output:", err)
		}
	}
	sort.Slice(historicOutputs, func(i, j int) bool {
		return bytes.Compare(historicOutputs[i].id[:], historicOutputs[j].id[:]) < 0
	})
	for _, ho := range historicOutputs {
		err := dbPutHistoricOutput(tx, ho.id, ho.val)
		if err != nil {
			w.log.Severe("Could not update historic output:", err)
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		// Add to historic claim starts.
		// NOTE: it's never necessary to delete from the historic claim start set.
		if diff.Direction == modules.DiffApply {
			err := dbPutHistoricClaimStart(tx, diff.ID, diff.SiafundOutput.ClaimStart)
			if err != nil {
				w.log.Severe("Could not update historic claim start:", err)
			}
		}

		// Verify that the diff is relevant to the wallet.
		if !w.isWalletAddress(diff.SiafundOutput.UnlockHash) {
			continue
		}

		var err error
		if diff.Direction == modules.DiffApply {
			err = dbPutSiafundOutput(tx, diff.ID, diff.SiafundOutput)
		} else {
			err = dbDeleteSiafundOutput(tx, diff.ID)
		}
		if err != nil {
			w.log.Severe("Could not update siafund output:", err)
		}
	}
	for _, diff := range cc.SiafundPoolDiffs {
		if diff.Direction == modules.DiffApply {
			w.siafundPool = diff.Adjusted
		} else {
			w.siafundPool = diff.Previous
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
				if err := dbDeleteLastProcessedTransaction(tx); err != nil {
					w.log.Severe("Could not revert transaction:", err)
				}
			}
		}

		// Remove the miner payout transaction if applicable.
		for _, mp := range block.MinerPayouts {
			if w.isWalletAddress(mp.UnlockHash) {
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
func (w *Wallet) applyHistory(tx *bolt.Tx, applied []types.Block) error {
	for _, block := range applied {
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
			// Apply the miner payout transaction if applicable.
			minerPT := modules.ProcessedTransaction{
				Transaction:           types.Transaction{},
				TransactionID:         types.TransactionID(block.ID()),
				ConfirmationHeight:    consensusHeight,
				ConfirmationTimestamp: block.Timestamp,
			}
			for _, mp := range block.MinerPayouts {
				minerPT.Outputs = append(minerPT.Outputs, modules.ProcessedOutput{
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

			pt := modules.ProcessedTransaction{
				Transaction:           txn,
				TransactionID:         txn.ID(),
				ConfirmationHeight:    consensusHeight,
				ConfirmationTimestamp: block.Timestamp,
			}

			for _, sci := range txn.SiacoinInputs {
				val, err := dbGetHistoricOutput(tx, types.OutputID(sci.ParentID))
				if err != nil {
					return fmt.Errorf("could not get historic output: %v", err)
				}
				pt.Inputs = append(pt.Inputs, modules.ProcessedInput{
					FundType:       types.SpecifierSiacoinInput,
					WalletAddress:  w.isWalletAddress(sci.UnlockConditions.UnlockHash()),
					RelatedAddress: sci.UnlockConditions.UnlockHash(),
					Value:          val,
				})
			}

			for _, sco := range txn.SiacoinOutputs {
				pt.Outputs = append(pt.Outputs, modules.ProcessedOutput{
					FundType:       types.SpecifierSiacoinOutput,
					MaturityHeight: consensusHeight,
					WalletAddress:  w.isWalletAddress(sco.UnlockHash),
					RelatedAddress: sco.UnlockHash,
					Value:          sco.Value,
				})
			}

			for _, sfi := range txn.SiafundInputs {
				sfiValue, err := dbGetHistoricOutput(tx, types.OutputID(sfi.ParentID))
				if err != nil {
					return fmt.Errorf("could not get historic output: %v", err)
				}
				pt.Inputs = append(pt.Inputs, modules.ProcessedInput{
					FundType:       types.SpecifierSiafundInput,
					WalletAddress:  w.isWalletAddress(sfi.UnlockConditions.UnlockHash()),
					RelatedAddress: sfi.UnlockConditions.UnlockHash(),
					Value:          sfiValue,
				})
				startVal, err := dbGetHistoricClaimStart(tx, sfi.ParentID)
				if err != nil {
					return fmt.Errorf("could not get historic claim start: %v", err)
				}
				claimValue := w.siafundPool.Sub(startVal).Mul(sfiValue)
				pt.Outputs = append(pt.Outputs, modules.ProcessedOutput{
					FundType:       types.SpecifierClaimOutput,
					MaturityHeight: consensusHeight + types.MaturityDelay,
					WalletAddress:  w.isWalletAddress(sfi.UnlockConditions.UnlockHash()),
					RelatedAddress: sfi.ClaimUnlockHash,
					Value:          claimValue,
				})
			}

			for _, sfo := range txn.SiafundOutputs {
				pt.Outputs = append(pt.Outputs, modules.ProcessedOutput{
					FundType:       types.SpecifierSiafundOutput,
					MaturityHeight: consensusHeight,
					WalletAddress:  w.isWalletAddress(sfo.UnlockHash),
					RelatedAddress: sfo.UnlockHash,
					Value:          sfo.Value,
				})
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

// next: make global txn implicit everywhere
// also need exclusivity wrt consistency (can't report anything that isn't synced to disk)

// ProcessConsensusChange parses a consensus change to update the set of
// confirmed outputs known to the wallet.
func (w *Wallet) ProcessConsensusChange(cc modules.ConsensusChange) {
	if err := w.tg.Add(); err != nil {
		// The wallet should gracefully reject updates from the consensus set
		// or transaction pool that are sent after the wallet's Close method
		// has closed the wallet's ThreadGroup.
		return
	}
	defer w.tg.Done()
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.updateConfirmedSet(w.dbTx, cc); err != nil {
		w.log.Println("ERROR: failed to update confirmed set:", err)
	}
	if err := w.revertHistory(w.dbTx, cc.RevertedBlocks); err != nil {
		w.log.Println("ERROR: failed to revert consensus change:", err)
	}
	if err := w.applyHistory(w.dbTx, cc.AppliedBlocks); err != nil {
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
func (w *Wallet) ReceiveUpdatedUnconfirmedTransactions(txns []types.Transaction, cc modules.ConsensusChange) {
	if err := w.tg.Add(); err != nil {
		// Gracefully reject transactions if the wallet's Close method has
		// closed the wallet's ThreadGroup already.
		return
	}
	defer w.tg.Done()

	w.mu.Lock()
	defer w.mu.Unlock()

	// record the historic outputs.
	// NOTE: it's safe to add unconfirmed outputs to the historic output set.
	for _, diff := range cc.SiacoinOutputDiffs {
		if diff.Direction == modules.DiffApply {
			err := dbPutHistoricOutput(w.dbTx, types.OutputID(diff.ID), diff.SiacoinOutput.Value)
			if err != nil {
				w.log.Severe("Could not add historic output:", err)
			}
		}
	}

	w.unconfirmedProcessedTransactions = nil
	for _, txn := range txns {
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
			TransactionID:         txn.ID(),
			ConfirmationHeight:    types.BlockHeight(math.MaxUint64),
			ConfirmationTimestamp: types.Timestamp(math.MaxUint64),
		}
		for _, sci := range txn.SiacoinInputs {
			val, err := dbGetHistoricOutput(w.dbTx, types.OutputID(sci.ParentID))
			if err != nil {
				w.log.Println("ERROR: could not get historic output:", err)
			}
			pt.Inputs = append(pt.Inputs, modules.ProcessedInput{
				FundType:       types.SpecifierSiacoinInput,
				WalletAddress:  w.isWalletAddress(sci.UnlockConditions.UnlockHash()),
				RelatedAddress: sci.UnlockConditions.UnlockHash(),
				Value:          val,
			})
		}
		for _, sco := range txn.SiacoinOutputs {
			pt.Outputs = append(pt.Outputs, modules.ProcessedOutput{
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
