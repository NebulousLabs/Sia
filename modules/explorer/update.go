package explorer

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// ProcessConsensusChange follows the most recent changes to the consensus set,
// including parsing new blocks and updating the utxo sets.
func (e *Explorer) ProcessConsensusChange(cc modules.ConsensusChange) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Update cumulative stats for reverted blocks.
	for _, block := range cc.RevertedBlocks {
		// Delete the block from the list of active blocks.
		e.blockchainHeight -= 1
		delete(e.blockHashes, block.ID())
		delete(e.transactionHashes, types.TransactionID(block.ID())) // Miner payouts are a transaction.

		// Catalog the removed miner payouts.
		for j, payout := range block.MinerPayouts {
			delete(e.siacoinOutputIDs[block.MinerPayoutID(uint64(j))], types.TransactionID(block.ID()))
			delete(e.unlockHashes[payout.UnlockHash], types.TransactionID(block.ID()))
		}

		// Update cumulative stats for reverted transcations.
		for _, txn := range block.Transactions {
			// Add the transction to the list of active transactions.
			e.transactionCount--
			delete(e.transactionHashes, txn.ID())

			for _, sci := range txn.SiacoinInputs {
				delete(e.siacoinOutputIDs[sci.ParentID], txn.ID())
				delete(e.unlockHashes[sci.UnlockConditions.UnlockHash()], txn.ID())
				e.siacoinInputCount--
			}
			for k, sco := range txn.SiacoinOutputs {
				delete(e.siacoinOutputIDs[txn.SiacoinOutputID(uint64(k))], txn.ID())
				delete(e.unlockHashes[sco.UnlockHash], txn.ID())
				e.siacoinOutputCount--
			}
			for k, fc := range txn.FileContracts {
				fcid := txn.FileContractID(uint64(k))
				delete(e.fileContractIDs[fcid], txn.ID())
				delete(e.unlockHashes[fc.UnlockHash], txn.ID())
				for l, sco := range fc.ValidProofOutputs {
					delete(e.siacoinOutputIDs[fcid.StorageProofOutputID(types.ProofValid, uint64(l))], txn.ID())
					delete(e.unlockHashes[sco.UnlockHash], txn.ID())
				}
				for l, sco := range fc.MissedProofOutputs {
					delete(e.siacoinOutputIDs[fcid.StorageProofOutputID(types.ProofMissed, uint64(l))], txn.ID())
					delete(e.unlockHashes[sco.UnlockHash], txn.ID())
				}
				e.fileContractCount--
				e.totalContractCost = e.totalContractCost.Sub(fc.Payout)
				e.totalContractSize = e.totalContractSize.Sub(types.NewCurrency64(fc.FileSize))
			}
			for _, fcr := range txn.FileContractRevisions {
				delete(e.fileContractIDs[fcr.ParentID], txn.ID())
				delete(e.unlockHashes[fcr.UnlockConditions.UnlockHash()], txn.ID())
				delete(e.unlockHashes[fcr.NewUnlockHash], txn.ID())
				for l, sco := range fcr.NewValidProofOutputs {
					delete(e.siacoinOutputIDs[fcr.ParentID.StorageProofOutputID(types.ProofValid, uint64(l))], txn.ID())
					delete(e.unlockHashes[sco.UnlockHash], txn.ID())
				}
				for l, sco := range fcr.NewMissedProofOutputs {
					delete(e.siacoinOutputIDs[fcr.ParentID.StorageProofOutputID(types.ProofMissed, uint64(l))], txn.ID())
					delete(e.unlockHashes[sco.UnlockHash], txn.ID())
				}
				e.fileContractRevisionCount--
				e.totalContractSize = e.totalContractSize.Sub(types.NewCurrency64(fcr.NewFileSize))
				e.totalRevisionVolume = e.totalRevisionVolume.Sub(types.NewCurrency64(fcr.NewFileSize))
			}
			for _, sp := range txn.StorageProofs {
				delete(e.fileContractIDs[sp.ParentID], txn.ID())
				e.storageProofCount--
			}
			for _, sfi := range txn.SiafundInputs {
				delete(e.siafundOutputIDs[sfi.ParentID], txn.ID())
				delete(e.unlockHashes[sfi.UnlockConditions.UnlockHash()], txn.ID())
				delete(e.unlockHashes[sfi.ClaimUnlockHash], txn.ID())
				e.siafundInputCount--
			}
			for k, sfo := range txn.SiafundOutputs {
				delete(e.siafundOutputIDs[txn.SiafundOutputID(uint64(k))], txn.ID())
				delete(e.unlockHashes[sfo.UnlockHash], txn.ID())
				e.siafundOutputCount--
			}
			for _ = range txn.MinerFees {
				e.minerFeeCount--
			}
			for _ = range txn.ArbitraryData {
				e.arbitraryDataCount--
			}
			for _ = range txn.TransactionSignatures {
				e.transactionSignatureCount--
			}
		}
	}

	// Update cumulative stats for applied blocks.
	for _, block := range cc.AppliedBlocks {
		// Add the block to the list of active blocks.
		e.blockchainHeight++
		e.blockHashes[block.ID()] = e.blockchainHeight
		e.transactionHashes[types.TransactionID(block.ID())] = e.blockchainHeight // Miner payouts are a transaciton.

		// Catalog the new miner payouts.
		for j, payout := range block.MinerPayouts {
			_, exists := e.siacoinOutputIDs[block.MinerPayoutID(uint64(j))]
			if !exists {
				e.siacoinOutputIDs[block.MinerPayoutID(uint64(j))] = make(map[types.TransactionID]struct{})
			}
			e.siacoinOutputIDs[block.MinerPayoutID(uint64(j))][types.TransactionID(block.ID())] = struct{}{}
			_, exists = e.unlockHashes[payout.UnlockHash]
			if !exists {
				e.unlockHashes[payout.UnlockHash] = make(map[types.TransactionID]struct{})
			}
			e.unlockHashes[payout.UnlockHash][types.TransactionID(block.ID())] = struct{}{}
		}

		// Update cumulative stats for applied transactions.
		for _, txn := range block.Transactions {
			// Add the transaction to the list of active transactions.
			e.transactionCount++
			e.transactionHashes[txn.ID()] = e.blockchainHeight

			for _, sci := range txn.SiacoinInputs {
				_, exists := e.siacoinOutputIDs[sci.ParentID]
				if !exists {
					e.siacoinOutputIDs[sci.ParentID] = make(map[types.TransactionID]struct{})
				}
				e.siacoinOutputIDs[sci.ParentID][txn.ID()] = struct{}{}
				_, exists = e.unlockHashes[sci.UnlockConditions.UnlockHash()]
				if !exists {
					e.unlockHashes[sci.UnlockConditions.UnlockHash()] = make(map[types.TransactionID]struct{})
				}
				e.unlockHashes[sci.UnlockConditions.UnlockHash()][txn.ID()] = struct{}{}
				e.siacoinInputCount++
			}
			for j, sco := range txn.SiacoinOutputs {
				_, exists := e.siacoinOutputIDs[txn.SiacoinOutputID(uint64(j))]
				if !exists {
					e.siacoinOutputIDs[txn.SiacoinOutputID(uint64(j))] = make(map[types.TransactionID]struct{})
				}
				e.siacoinOutputIDs[txn.SiacoinOutputID(uint64(j))][txn.ID()] = struct{}{}
				_, exists = e.unlockHashes[sco.UnlockHash]
				if !exists {
					e.unlockHashes[sco.UnlockHash] = make(map[types.TransactionID]struct{})
				}
				e.unlockHashes[sco.UnlockHash][txn.ID()] = struct{}{}
				e.siacoinOutputCount++
			}
			for _, fc := range txn.FileContracts {
				e.fileContractCount++
				e.totalContractCost = e.totalContractCost.Add(fc.Payout)
				e.totalContractSize = e.totalContractSize.Add(types.NewCurrency64(fc.FileSize))
			}
			for _, fcr := range txn.FileContractRevisions {
				e.fileContractRevisionCount++
				e.totalContractSize = e.totalContractSize.Add(types.NewCurrency64(fcr.NewFileSize))
				e.totalRevisionVolume = e.totalRevisionVolume.Add(types.NewCurrency64(fcr.NewFileSize))
			}
			for _ = range txn.StorageProofs {
				e.storageProofCount++
			}
			for _ = range txn.SiafundInputs {
				e.siafundInputCount++
			}
			for _ = range txn.SiafundOutputs {
				e.siafundOutputCount++
			}
			for _ = range txn.MinerFees {
				e.minerFeeCount++
			}
			for _ = range txn.ArbitraryData {
				e.arbitraryDataCount++
			}
			for _ = range txn.TransactionSignatures {
				e.transactionSignatureCount++
			}
		}
	}

	// Compute the changes in the active set.
	for _, diff := range cc.FileContractDiffs {
		if diff.Direction == modules.DiffApply {
			e.activeContractCount += 1
			e.activeContractCost = e.activeContractCost.Add(diff.FileContract.Payout)
			e.activeContractSize = e.activeContractSize.Add(types.NewCurrency64(diff.FileContract.FileSize))
		} else {
			e.activeContractCount -= 1
			e.activeContractCost = e.activeContractCost.Sub(diff.FileContract.Payout)
			e.activeContractSize = e.activeContractSize.Sub(types.NewCurrency64(diff.FileContract.FileSize))
		}
	}

	// Set the id of the current block.
	e.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID()
}
