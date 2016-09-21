package wallet

import (
	"fmt"
	"math"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// isWalletAddress is a helper function that checks if an UnlockHash is
// derived from one of the wallet's spendable keys.
func (w *Wallet) isWalletAddress(uh types.UnlockHash) bool {
	_, exists := w.keys[uh]
	return exists
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
			err = dbPutSiacoinOutput(tx, diff.ID, diff.SiacoinOutput)
		} else {
			err = dbDeleteSiacoinOutput(tx, diff.ID)
		}
		if err != nil {
			return err
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
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
			return err
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
					return err
				}
			}
		}

		// Remove the miner payout transaction if applicable.
		for _, mp := range block.MinerPayouts {
			if w.isWalletAddress(mp.UnlockHash) {
				if err := dbDeleteLastProcessedTransaction(tx); err != nil {
					return err
				}
				break
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
		for i, mp := range block.MinerPayouts {
			relevant = relevant || w.isWalletAddress(mp.UnlockHash)
			err := dbPutHistoricOutput(tx, types.OutputID(block.MinerPayoutID(uint64(i))), mp.Value)
			if err != nil {
				return fmt.Errorf("could not put historic output: %v", err)
			}
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
			for i, sco := range txn.SiacoinOutputs {
				relevant = relevant || w.isWalletAddress(sco.UnlockHash)
				err := dbPutHistoricOutput(tx, types.OutputID(txn.SiacoinOutputID(uint64(i))), sco.Value)
				if err != nil {
					return fmt.Errorf("could not put historic output: %v", err)
				}
			}
			for _, sfi := range txn.SiafundInputs {
				relevant = relevant || w.isWalletAddress(sfi.UnlockConditions.UnlockHash())
			}

			for i, sfo := range txn.SiafundOutputs {
				relevant = relevant || w.isWalletAddress(sfo.UnlockHash)
				id := txn.SiafundOutputID(uint64(i))
				err := dbPutHistoricOutput(tx, types.OutputID(id), sfo.Value)
				if err != nil {
					return fmt.Errorf("could not put historic output: %v", err)
				}
				err = dbPutHistoricClaimStart(tx, id, sfo.ClaimStart)
				if err != nil {
					return fmt.Errorf("could not put historic claim start: %v", err)
				}
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
	err := w.db.Update(func(tx *bolt.Tx) error {
		if err := w.updateConfirmedSet(tx, cc); err != nil {
			return err
		}
		if err := w.revertHistory(tx, cc.RevertedBlocks); err != nil {
			return err
		}
		if err := w.applyHistory(tx, cc.AppliedBlocks); err != nil {
			return err
		}
		// update the ConsensusChangeID
		return dbPutConsensusChangeID(tx, cc.ID)
	})
	if err != nil {
		w.log.Println("ERROR: failed to add consensus change:", err)
	}
}

// ReceiveUpdatedUnconfirmedTransactions updates the wallet's unconfirmed
// transaction set.
func (w *Wallet) ReceiveUpdatedUnconfirmedTransactions(txns []types.Transaction, _ modules.ConsensusChange) {
	if err := w.tg.Add(); err != nil {
		// Gracefully reject transactions if the wallet's Close method has
		// closed the wallet's ThreadGroup already.
		return
	}
	defer w.tg.Done()

	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.db.Update(func(tx *bolt.Tx) error {
		w.unconfirmedProcessedTransactions = nil
		for _, txn := range txns {
			// determine whether transaction is relevant to the wallet
			relevant := false
			for _, sci := range txn.SiacoinInputs {
				relevant = relevant || w.isWalletAddress(sci.UnlockConditions.UnlockHash())
			}
			for i, sco := range txn.SiacoinOutputs {
				relevant = relevant || w.isWalletAddress(sco.UnlockHash)
				err := dbPutHistoricOutput(tx, types.OutputID(txn.SiacoinOutputID(uint64(i))), sco.Value)
				if err != nil {
					return fmt.Errorf("could not put historic output: %v", err)
				}
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
		return nil
	})
	if err != nil {
		w.log.Println("ERROR: failed to add unconfirmed transactions:", err)
	}
}
