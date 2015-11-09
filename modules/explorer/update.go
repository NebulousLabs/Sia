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
		bid := block.ID()
		tbid := types.TransactionID(bid)
		e.blockchainHeight -= 1
		delete(e.blockHashes, bid)
		delete(e.transactionHashes, tbid)// Miner payouts are a transaction.

		// Catalog the removed miner payouts.
		for j, payout := range block.MinerPayouts {
			scoid := block.MinerPayoutID(uint64(j))
			delete(e.siacoinOutputIDs[scoid], tbid)
			delete(e.unlockHashes[payout.UnlockHash], tbid)
		}

		// Update cumulative stats for reverted transcations.
		for _, txn := range block.Transactions {
			// Add the transction to the list of active transactions.
			txid := txn.ID()
			e.transactionCount--
			delete(e.transactionHashes, txid)

			for _, sci := range txn.SiacoinInputs {
				delete(e.siacoinOutputIDs[sci.ParentID], txid)
				delete(e.unlockHashes[sci.UnlockConditions.UnlockHash()], txid)
				e.siacoinInputCount--
			}
			for k, sco := range txn.SiacoinOutputs {
				delete(e.siacoinOutputIDs[txn.SiacoinOutputID(uint64(k))], txid)
				delete(e.unlockHashes[sco.UnlockHash], txid)
				e.siacoinOutputCount--
			}
			for k, fc := range txn.FileContracts {
				fcid := txn.FileContractID(uint64(k))
				delete(e.fileContractIDs[fcid], txid)
				delete(e.unlockHashes[fc.UnlockHash], txid)
				for l, sco := range fc.ValidProofOutputs {
					scoid := fcid.StorageProofOutputID(types.ProofValid, uint64(l))
					delete(e.siacoinOutputIDs[scoid], txid)
					delete(e.unlockHashes[sco.UnlockHash], txid)
				}
				for l, sco := range fc.MissedProofOutputs {
					scoid := fcid.StorageProofOutputID(types.ProofMissed, uint64(l))
					delete(e.siacoinOutputIDs[scoid], txid)
					delete(e.unlockHashes[sco.UnlockHash], txid)
				}
				e.fileContractCount--
				e.totalContractCost = e.totalContractCost.Sub(fc.Payout)
				e.totalContractSize = e.totalContractSize.Sub(types.NewCurrency64(fc.FileSize))
			}
			for _, fcr := range txn.FileContractRevisions {
				delete(e.fileContractIDs[fcr.ParentID], txid)
				delete(e.unlockHashes[fcr.UnlockConditions.UnlockHash()], txid)
				delete(e.unlockHashes[fcr.NewUnlockHash], txid)
				for l, sco := range fcr.NewValidProofOutputs {
					scoid := fcr.ParentID.StorageProofOutputID(types.ProofValid, uint64(l))
					delete(e.siacoinOutputIDs[scoid], txid)
					delete(e.unlockHashes[sco.UnlockHash], txid)
				}
				for l, sco := range fcr.NewMissedProofOutputs {
					scoid := fcr.ParentID.StorageProofOutputID(types.ProofMissed, uint64(l))
					delete(e.siacoinOutputIDs[scoid], txid)
					delete(e.unlockHashes[sco.UnlockHash], txid)
				}
				e.fileContractRevisionCount--
				e.totalContractSize = e.totalContractSize.Sub(types.NewCurrency64(fcr.NewFileSize))
				e.totalRevisionVolume = e.totalRevisionVolume.Sub(types.NewCurrency64(fcr.NewFileSize))
			}
			for _, sp := range txn.StorageProofs {
				delete(e.fileContractIDs[sp.ParentID], txid)
				e.storageProofCount--
			}
			for _, sfi := range txn.SiafundInputs {
				delete(e.siafundOutputIDs[sfi.ParentID], txid)
				delete(e.unlockHashes[sfi.UnlockConditions.UnlockHash()], txid)
				delete(e.unlockHashes[sfi.ClaimUnlockHash], txid)
				e.siafundInputCount--
			}
			for k, sfo := range txn.SiafundOutputs {
				sfoid := txn.SiafundOutputID(uint64(k))
				delete(e.siafundOutputIDs[sfoid], txid)
				delete(e.unlockHashes[sfo.UnlockHash], txid)
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
		bid := block.ID()
		tbid := types.TransactionID(bid)
		e.blockchainHeight++
		e.blockHashes[bid] = e.blockchainHeight
		e.transactionHashes[tbid] = e.blockchainHeight // Miner payouts are a transaciton.

		// Catalog the new miner payouts.
		for j, payout := range block.MinerPayouts {
			scoid := block.MinerPayoutID(uint64(j))
			_, exists := e.siacoinOutputIDs[scoid]
			if !exists {
				e.siacoinOutputIDs[scoid] = make(map[types.TransactionID]struct{})
			}
			e.siacoinOutputIDs[scoid][tbid] = struct{}{}
			_, exists = e.unlockHashes[payout.UnlockHash]
			if !exists {
				e.unlockHashes[payout.UnlockHash] = make(map[types.TransactionID]struct{})
			}
			e.unlockHashes[payout.UnlockHash][tbid] = struct{}{}
		}

		// Update cumulative stats for applied transactions.
		for _, txn := range block.Transactions {
			// Add the transaction to the list of active transactions.
			txid := txn.ID()
			e.transactionCount++
			e.transactionHashes[txid] = e.blockchainHeight

			for _, sci := range txn.SiacoinInputs {
				_, exists := e.siacoinOutputIDs[sci.ParentID]
				if !exists {
					panic("siacoin input without siacoin output")
				}
				e.siacoinOutputIDs[sci.ParentID][txid] = struct{}{}
				_, exists = e.unlockHashes[sci.UnlockConditions.UnlockHash()]
				if !exists {
					panic("unlock conditions without a parent unlock hash")
				}
				e.unlockHashes[sci.UnlockConditions.UnlockHash()][txid] = struct{}{}
				e.siacoinInputCount++
			}
			for j, sco := range txn.SiacoinOutputs {
				scoid := txn.SiacoinOutputID(uint64(j))
				_, exists := e.siacoinOutputIDs[scoid]
				if !exists {
					e.siacoinOutputIDs[scoid] = make(map[types.TransactionID]struct{})
				}
				e.siacoinOutputIDs[scoid][txid] = struct{}{}
				_, exists = e.unlockHashes[sco.UnlockHash]
				if !exists {
					e.unlockHashes[sco.UnlockHash] = make(map[types.TransactionID]struct{})
				}
				e.unlockHashes[sco.UnlockHash][txn.ID()] = struct{}{}
				e.siacoinOutputCount++
			}
			for k, fc := range txn.FileContracts {
				fcid := txn.FileContractID(uint64(k))
				_, exists := e.fileContractIDs[fcid]
				if !exists {
					e.fileContractIDs[fcid] = make(map[types.TransactionID]struct{})
				}
				e.fileContractIDs[fcid][txid] = struct{}{}
				_, exists = e.unlockHashes[fc.UnlockHash]
				if !exists {
					e.unlockHashes[fc.UnlockHash] = make(map[types.TransactionID]struct{})
				}
				e.unlockHashes[fc.UnlockHash][txid] = struct{}{}
				for l, sco := range fc.ValidProofOutputs {
					scoid := fcid.StorageProofOutputID(types.ProofValid, uint64(l))
					_, exists = e.siacoinOutputIDs[scoid]
					if !exists {
						e.siacoinOutputIDs[scoid] = make(map[types.TransactionID]struct{})
					}
					e.siacoinOutputIDs[scoid][txid] = struct{}{}
					_, exists = e.unlockHashes[sco.UnlockHash]
					if !exists {
						e.unlockHashes[sco.UnlockHash] = make(map[types.TransactionID]struct{})
					}
					e.unlockHashes[sco.UnlockHash][txid] = struct{}{}
				}
				for l, sco := range fc.MissedProofOutputs {
					scoid := fcid.StorageProofOutputID(types.ProofMissed, uint64(l))
					_, exists = e.siacoinOutputIDs[scoid]
					if !exists {
						e.siacoinOutputIDs[scoid] = make(map[types.TransactionID]struct{})
					}
					e.siacoinOutputIDs[scoid][txid] = struct{}{}
					_, exists = e.unlockHashes[sco.UnlockHash]
					if !exists {
						e.unlockHashes[sco.UnlockHash] = make(map[types.TransactionID]struct{})
					}
					e.unlockHashes[sco.UnlockHash][txid] = struct{}{}
				}
				e.fileContractCount++
				e.totalContractCost = e.totalContractCost.Add(fc.Payout)
				e.totalContractSize = e.totalContractSize.Add(types.NewCurrency64(fc.FileSize))
			}
			for _, fcr := range txn.FileContractRevisions {
				_, exists := e.fileContractIDs[fcr.ParentID]
				if !exists {
					panic("revision without entry in file contract list")
				}
				e.fileContractIDs[fcr.ParentID][txid] = struct{}{}
				_, exists = e.unlockHashes[fcr.UnlockConditions.UnlockHash()]
				if !exists {
					panic("unlock conditions without unlock hash")
				}
				e.unlockHashes[fcr.UnlockConditions.UnlockHash()][txid] = struct{}{}
				_, exists = e.unlockHashes[fcr.NewUnlockHash]
				if !exists {
					e.unlockHashes[fcr.NewUnlockHash] = make(map[types.TransactionID]struct{})
				}
				e.unlockHashes[fcr.NewUnlockHash][txid] = struct{}{}
				for l, sco := range fcr.NewValidProofOutputs {
					scoid := fcr.ParentID.StorageProofOutputID(types.ProofValid, uint64(l))
					_, exists = e.siacoinOutputIDs[scoid]
					if !exists {
						e.siacoinOutputIDs[scoid] = make(map[types.TransactionID]struct{})
					}
					e.siacoinOutputIDs[scoid][txid] = struct{}{}
					_, exists = e.unlockHashes[sco.UnlockHash]
					if !exists {
						e.unlockHashes[sco.UnlockHash] = make(map[types.TransactionID]struct{})
					}
					e.unlockHashes[sco.UnlockHash][txid] = struct{}{}
				}
				for l, sco := range fcr.NewMissedProofOutputs {
					scoid := fcr.ParentID.StorageProofOutputID(types.ProofMissed, uint64(l))
					_, exists = e.siacoinOutputIDs[scoid]
					if !exists {
						e.siacoinOutputIDs[scoid] = make(map[types.TransactionID]struct{})
					}
					e.siacoinOutputIDs[scoid][txid] = struct{}{}
					_, exists = e.unlockHashes[sco.UnlockHash]
					if !exists {
						e.unlockHashes[sco.UnlockHash] = make(map[types.TransactionID]struct{})
					}
					e.unlockHashes[sco.UnlockHash][txid] = struct{}{}
				}
				e.fileContractRevisionCount++
				e.totalContractSize = e.totalContractSize.Add(types.NewCurrency64(fcr.NewFileSize))
				e.totalRevisionVolume = e.totalRevisionVolume.Add(types.NewCurrency64(fcr.NewFileSize))
			}
			for _, sp := range txn.StorageProofs {
				_, exists := e.fileContractIDs[sp.ParentID]
				if !exists {
					panic("storage proof without file contract parent")
				}
				e.fileContractIDs[sp.ParentID][txid] = struct{}{}
				e.storageProofCount++
			}
			for _, sfi := range txn.SiafundInputs {
				_, exists := e.siafundOutputIDs[sfi.ParentID]
				if !exists {
					panic("siafund input without corresponding output")
				}
				e.siafundOutputIDs[sfi.ParentID][txid] = struct{}{}
				_, exists = e.unlockHashes[sfi.UnlockConditions.UnlockHash()]
				if !exists {
					panic("unlock conditions without unlock hash")
				}
				e.unlockHashes[sfi.UnlockConditions.UnlockHash()][txid] = struct{}{}
				_, exists = e.unlockHashes[sfi.ClaimUnlockHash]
				if !exists {
					e.unlockHashes[sfi.ClaimUnlockHash] = make(map[types.TransactionID]struct{})
				}
				e.unlockHashes[sfi.ClaimUnlockHash][txid] = struct{}{}
				e.siafundInputCount++
			}
			for k, sfo := range txn.SiafundOutputs {
				sfoid := txn.SiafundOutputID(uint64(k))
				_, exists := e.siafundOutputIDs[sfoid]
				if !exists {
					e.siafundOutputIDs[sfoid] = make(map[types.TransactionID]struct{})
				}
				e.siafundOutputIDs[sfoid][txid] = struct{}{}
				_, exists = e.unlockHashes[sfo.UnlockHash]
				if !exists {
					e.unlockHashes[sfo.UnlockHash] = make(map[types.TransactionID]struct{})
				}
				e.unlockHashes[sfo.UnlockHash][txid] = struct{}{}
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
