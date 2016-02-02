package explorer

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// ProcessConsensusChange follows the most recent changes to the consensus set,
// including parsing new blocks and updating the utxo sets.
func (e *Explorer) ProcessConsensusChange(cc modules.ConsensusChange) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(cc.AppliedBlocks) == 0 {
		build.Critical("Explorer.ProcessConsensusChange called with a ConensusChange that has no AppliedBlocks")
	}

	// Update cumulative stats for reverted blocks.
	for _, block := range cc.RevertedBlocks {
		bid := block.ID()
		tbid := types.TransactionID(bid)

		// Update all of the explorer statistics.
		e.blockchainHeight--
		e.target = e.blockTargets[bid]
		e.timestamp = block.Timestamp
		e.blocksDifficulty = e.blocksDifficulty.SubtractDifficulties(e.target)
		if e.blockchainHeight > hashrateEstimationBlocks {
			e.blocksDifficulty = e.blocksDifficulty.AddDifficulties(e.historicFacts[e.blockchainHeight-hashrateEstimationBlocks].target)
			secondsPassed := e.timestamp - e.historicFacts[e.blockchainHeight-hashrateEstimationBlocks].timestamp
			e.estimatedHashrate = e.blocksDifficulty.Difficulty().Div(types.NewCurrency64(uint64(secondsPassed)))
		}

		// Delete the block from the list of active blocks.
		delete(e.blockHashes, bid)
		delete(e.blockTargets, bid)
		delete(e.transactionHashes, tbid) // Miner payouts are a transaction.

		// Catalog the removed miner payouts.
		for j, payout := range block.MinerPayouts {
			scoid := block.MinerPayoutID(uint64(j))
			delete(e.siacoinOutputIDs[scoid], tbid)
			delete(e.unlockHashes[payout.UnlockHash], tbid)
			e.minerPayoutCount--
		}

		// Update cumulative stats for reverted transcations.
		for _, txn := range block.Transactions {
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
				// Remove the file contract revision from the revision chain.
				e.fileContractHistories[fcr.ParentID].revisions = e.fileContractHistories[fcr.ParentID].revisions[:len(e.fileContractHistories[fcr.ParentID].revisions)-1]
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
	// Delete all of the block facts for the reverted blocks.
	e.historicFacts = e.historicFacts[:len(e.historicFacts)-len(cc.RevertedBlocks)]

	// Update cumulative stats for applied blocks.
	for _, block := range cc.AppliedBlocks {
		// Add the block to the list of active blocks.
		bid := block.ID()
		tbid := types.TransactionID(bid)
		e.currentBlock = bid
		e.blockchainHeight++
		var exists bool
		e.target, exists = e.cs.ChildTarget(block.ParentID)
		if !exists {
			e.target = types.RootTarget
		}
		e.timestamp = block.Timestamp
		if e.blockchainHeight > types.MaturityDelay {
			e.maturityTimestamp = e.historicFacts[e.blockchainHeight-types.MaturityDelay].timestamp
		}
		e.blocksDifficulty = e.blocksDifficulty.AddDifficulties(e.target)
		if e.blockchainHeight > hashrateEstimationBlocks {
			e.blocksDifficulty = e.blocksDifficulty.SubtractDifficulties(e.historicFacts[e.blockchainHeight-hashrateEstimationBlocks].target)
			secondsPassed := e.timestamp - e.historicFacts[e.blockchainHeight-hashrateEstimationBlocks].timestamp
			e.estimatedHashrate = e.blocksDifficulty.Difficulty().Div(types.NewCurrency64(uint64(secondsPassed)))
		}
		e.totalCoins = types.CalculateNumSiacoins(e.blockchainHeight)

		e.blockHashes[bid] = e.blockchainHeight
		e.transactionHashes[tbid] = e.blockchainHeight // Miner payouts are a transaciton.
		e.blockTargets[bid] = e.target

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
			e.minerPayoutCount++
		}

		// Update cumulative stats for applied transactions.
		for _, txn := range block.Transactions {
			// Add the transaction to the list of active transactions.
			txid := txn.ID()
			e.transactionCount++
			e.transactionHashes[txid] = e.blockchainHeight

			for _, sci := range txn.SiacoinInputs {
				_, exists := e.siacoinOutputIDs[sci.ParentID]
				if build.DEBUG && !exists {
					panic("siacoin input without siacoin output")
				} else if !exists {
					e.siacoinOutputIDs[sci.ParentID] = make(map[types.TransactionID]struct{})
				}
				e.siacoinOutputIDs[sci.ParentID][txid] = struct{}{}
				_, exists = e.unlockHashes[sci.UnlockConditions.UnlockHash()]
				if build.DEBUG && !exists {
					panic("unlock conditions without a parent unlock hash")
				} else if !exists {
					e.unlockHashes[sci.UnlockConditions.UnlockHash()] = make(map[types.TransactionID]struct{})
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
				e.siacoinOutputs[scoid] = sco
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
				e.fileContractHistories[fcid] = &fileContractHistory{contract: fc}
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
				if build.DEBUG && !exists {
					panic("revision without entry in file contract list")
				} else if !exists {
					e.fileContractIDs[fcr.ParentID] = make(map[types.TransactionID]struct{})
				}
				e.fileContractIDs[fcr.ParentID][txid] = struct{}{}
				_, exists = e.unlockHashes[fcr.UnlockConditions.UnlockHash()]
				if build.DEBUG && !exists {
					panic("unlock conditions without unlock hash")
				} else if !exists {
					e.unlockHashes[fcr.UnlockConditions.UnlockHash()] = make(map[types.TransactionID]struct{})
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
				e.fileContractHistories[fcr.ParentID].revisions = append(e.fileContractHistories[fcr.ParentID].revisions, fcr)
			}
			for _, sp := range txn.StorageProofs {
				_, exists := e.fileContractIDs[sp.ParentID]
				if build.DEBUG && !exists {
					panic("storage proof without file contract parent")
				} else if !exists {
					e.fileContractIDs[sp.ParentID] = make(map[types.TransactionID]struct{})
				}
				e.fileContractIDs[sp.ParentID][txid] = struct{}{}
				e.fileContractHistories[sp.ParentID].storageProof = sp
				e.storageProofCount++
			}
			for _, sfi := range txn.SiafundInputs {
				_, exists := e.siafundOutputIDs[sfi.ParentID]
				if build.DEBUG && !exists {
					panic("siafund input without corresponding output")
				} else if !exists {
					e.siafundOutputIDs[sfi.ParentID] = make(map[types.TransactionID]struct{})
				}
				e.siafundOutputIDs[sfi.ParentID][txid] = struct{}{}
				_, exists = e.unlockHashes[sfi.UnlockConditions.UnlockHash()]
				if build.DEBUG && !exists {
					panic("unlock conditions without unlock hash")
				} else if !exists {
					e.unlockHashes[sfi.UnlockConditions.UnlockHash()] = make(map[types.TransactionID]struct{})
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
				e.siafundOutputs[sfoid] = sfo
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

		// Set the current block and copy over the historic facts.
		e.historicFacts = append(e.historicFacts, e.blockFacts)
	}

	// Compute the changes in the active set. Note, because this is calculated
	// at the end instead of in a loop, the historic facts may contain
	// inaccuracies about the active set. This should not be a problem except
	// for large reorgs.
	for _, diff := range cc.FileContractDiffs {
		if diff.Direction == modules.DiffApply {
			e.activeContractCount++
			e.activeContractCost = e.activeContractCost.Add(diff.FileContract.Payout)
			e.activeContractSize = e.activeContractSize.Add(types.NewCurrency64(diff.FileContract.FileSize))
		} else {
			e.activeContractCount--
			e.activeContractCost = e.activeContractCost.Sub(diff.FileContract.Payout)
			e.activeContractSize = e.activeContractSize.Sub(types.NewCurrency64(diff.FileContract.FileSize))
		}
	}
}
