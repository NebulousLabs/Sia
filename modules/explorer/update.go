package explorer

import (
	"fmt"
	"math"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/bolt"
)

func (e *Explorer) ReceiveUpdatedUnconfirmedTransactions(diff *modules.TransactionPoolDiff) {
	e.mu.Lock()
	defer e.mu.Unlock()

	droppedTransactions := make(map[types.TransactionID]struct{})
	for i := range diff.RevertedTransactions {
		txids := e.unconfirmedSets[diff.RevertedTransactions[i]]
		for i := range txids {
			droppedTransactions[txids[i]] = struct{}{}
		}
		delete(e.unconfirmedSets, diff.RevertedTransactions[i])
	}

	if len(droppedTransactions) != 0 {
		newUPT := make([]modules.ProcessedTransaction, 0, len(e.unconfirmedProcessedTransactions))
		for _, txn := range e.unconfirmedProcessedTransactions {
			_, exists := droppedTransactions[txn.TransactionID]
			if !exists {
				// Transaction was not dropped, add it to the new unconfirmed
				// transactions.
				newUPT = append(newUPT, txn)
			}
		}
		// Set the unconfirmed preocessed transactions to the pruned set.
		e.unconfirmedProcessedTransactions = newUPT
	}

	// Scroll through all of the diffs and add any new transactions.
	for _, unconfirmedTxnSet := range diff.AppliedTransactions {
		e.unconfirmedSets[unconfirmedTxnSet.ID] = unconfirmedTxnSet.IDs

		spentSiacoinOutputs := make(map[types.SiacoinOutputID]types.SiacoinOutput)
		for _, scod := range unconfirmedTxnSet.Change.SiacoinOutputDiffs {
			if scod.Direction == modules.DiffRevert {
				spentSiacoinOutputs[scod.ID] = scod.SiacoinOutput
			}
		}

		// Add each transaction to our set of unconfirmed transactions.
		for i, txn := range unconfirmedTxnSet.Transactions {
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
					RelatedAddress: sci.UnlockConditions.UnlockHash(),
					Value:          spentSiacoinOutputs[sci.ParentID].Value,
				})
			}
			for i, sco := range txn.SiacoinOutputs {
				pt.Outputs = append(pt.Outputs, modules.ProcessedOutput{
					ID:             types.OutputID(txn.SiacoinOutputID(uint64(i))),
					FundType:       types.SpecifierSiacoinOutput,
					MaturityHeight: types.BlockHeight(math.MaxUint64),
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
			e.unconfirmedProcessedTransactions = append(e.unconfirmedProcessedTransactions, pt)
		}
	}
}

// ProcessConsensusChange follows the most recent changes to the consensus set,
// including parsing new blocks and updating the utxo sets.
func (e *Explorer) ProcessConsensusChange(cc modules.ConsensusChange) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(cc.AppliedBlocks) == 0 && build.DEBUG {
		build.Critical("Explorer.ProcessConsensusChange called with a ConsensusChange that has no AppliedBlocks")
	} else if len(cc.AppliedBlocks) == 0 {
		e.log.Printf("Explorer.ProcessConsensusChange called with a ConsensusChange that has no AppliedBlocks")
		return
	}

	err := e.db.Update(func(tx *bolt.Tx) (err error) {
		// use exception-style error handling to enable more concise update code
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%v", r)
			}
		}()

		// get starting block height
		e.persistMu.Lock()
		blockheight := e.persist.Height
		e.persistMu.Unlock()

		// Update cumulative stats for reverted blocks.
		for _, block := range cc.RevertedBlocks {
			bid := block.ID()
			tbid := types.TransactionID(bid)

			blockheight--
			dbRemoveBlockID(tx, bid)
			dbRemoveTransactionID(tx, tbid) // Miner payouts are a transaction

			target, exists := e.cs.ChildTarget(block.ParentID)
			if !exists {
				target = types.RootTarget
			}
			dbRemoveBlockTarget(tx, bid, target)

			// Remove miner payouts
			for j, payout := range block.MinerPayouts {
				scoid := block.MinerPayoutID(uint64(j))
				dbRemoveSiacoinOutputID(tx, scoid, tbid)
				dbRemoveUnlockHash(tx, payout.UnlockHash, tbid)
			}

			// Remove transactions
			for _, txn := range block.Transactions {
				txid := txn.ID()
				dbRemoveTransactionID(tx, txid)

				for _, sci := range txn.SiacoinInputs {
					dbRemoveSiacoinOutputID(tx, sci.ParentID, txid)
					dbRemoveUnlockHash(tx, sci.UnlockConditions.UnlockHash(), txid)
				}
				for k, sco := range txn.SiacoinOutputs {
					scoid := txn.SiacoinOutputID(uint64(k))
					dbRemoveSiacoinOutputID(tx, scoid, txid)
					dbRemoveUnlockHash(tx, sco.UnlockHash, txid)
					dbRemoveSiacoinOutput(tx, scoid)
				}
				for k, fc := range txn.FileContracts {
					fcid := txn.FileContractID(uint64(k))
					dbRemoveFileContractID(tx, fcid, txid)
					dbRemoveUnlockHash(tx, fc.UnlockHash, txid)
					for l, sco := range fc.ValidProofOutputs {
						scoid := fcid.StorageProofOutputID(types.ProofValid, uint64(l))
						dbRemoveSiacoinOutputID(tx, scoid, txid)
						dbRemoveUnlockHash(tx, sco.UnlockHash, txid)
					}
					for l, sco := range fc.MissedProofOutputs {
						scoid := fcid.StorageProofOutputID(types.ProofMissed, uint64(l))
						dbRemoveSiacoinOutputID(tx, scoid, txid)
						dbRemoveUnlockHash(tx, sco.UnlockHash, txid)
					}
					dbRemoveFileContract(tx, fcid)
				}
				for _, fcr := range txn.FileContractRevisions {
					dbRemoveFileContractID(tx, fcr.ParentID, txid)
					dbRemoveUnlockHash(tx, fcr.UnlockConditions.UnlockHash(), txid)
					dbRemoveUnlockHash(tx, fcr.NewUnlockHash, txid)
					for l, sco := range fcr.NewValidProofOutputs {
						scoid := fcr.ParentID.StorageProofOutputID(types.ProofValid, uint64(l))
						dbRemoveSiacoinOutputID(tx, scoid, txid)
						dbRemoveUnlockHash(tx, sco.UnlockHash, txid)
					}
					for l, sco := range fcr.NewMissedProofOutputs {
						scoid := fcr.ParentID.StorageProofOutputID(types.ProofMissed, uint64(l))
						dbRemoveSiacoinOutputID(tx, scoid, txid)
						dbRemoveUnlockHash(tx, sco.UnlockHash, txid)
					}
					// Remove the file contract revision from the revision chain.
					dbRemoveFileContractRevision(tx, fcr.ParentID)
				}
				for _, sp := range txn.StorageProofs {
					dbRemoveStorageProof(tx, sp.ParentID)
				}
				for _, sfi := range txn.SiafundInputs {
					dbRemoveSiafundOutputID(tx, sfi.ParentID, txid)
					dbRemoveUnlockHash(tx, sfi.UnlockConditions.UnlockHash(), txid)
					dbRemoveUnlockHash(tx, sfi.ClaimUnlockHash, txid)
				}
				for k, sfo := range txn.SiafundOutputs {
					sfoid := txn.SiafundOutputID(uint64(k))
					dbRemoveSiafundOutputID(tx, sfoid, txid)
					dbRemoveUnlockHash(tx, sfo.UnlockHash, txid)
				}
			}

			// remove the associated block facts
			dbRemoveBlockFacts(tx, bid)
		}

		// Update cumulative stats for applied blocks.
		for _, block := range cc.AppliedBlocks {
			bid := block.ID()
			tbid := types.TransactionID(bid)

			// special handling for genesis block
			if bid == types.GenesisID {
				dbAddGenesisBlock(tx)
				continue
			}

			blockheight++
			dbAddBlockID(tx, bid, blockheight)
			dbAddTransactionID(tx, tbid, blockheight) // Miner payouts are a transaction

			target, exists := e.cs.ChildTarget(block.ParentID)
			if !exists {
				target = types.RootTarget
			}
			dbAddBlockTarget(tx, bid, target)

			// Catalog the new miner payouts.
			for j, payout := range block.MinerPayouts {
				scoid := block.MinerPayoutID(uint64(j))
				dbAddSiacoinOutputID(tx, scoid, tbid)
				dbAddUnlockHash(tx, payout.UnlockHash, tbid)
			}

			// Update cumulative stats for applied transactions.
			for _, txn := range block.Transactions {
				// Add the transaction to the list of active transactions.
				txid := txn.ID()
				dbAddTransactionID(tx, txid, blockheight)

				for _, sci := range txn.SiacoinInputs {
					dbAddSiacoinOutputID(tx, sci.ParentID, txid)
					dbAddUnlockHash(tx, sci.UnlockConditions.UnlockHash(), txid)
				}
				for j, sco := range txn.SiacoinOutputs {
					scoid := txn.SiacoinOutputID(uint64(j))
					dbAddSiacoinOutputID(tx, scoid, txid)
					dbAddUnlockHash(tx, sco.UnlockHash, txid)
				}
				for k, fc := range txn.FileContracts {
					fcid := txn.FileContractID(uint64(k))
					dbAddFileContractID(tx, fcid, txid)
					dbAddUnlockHash(tx, fc.UnlockHash, txid)
					dbAddFileContract(tx, fcid, fc)
					for l, sco := range fc.ValidProofOutputs {
						scoid := fcid.StorageProofOutputID(types.ProofValid, uint64(l))
						dbAddSiacoinOutputID(tx, scoid, txid)
						dbAddUnlockHash(tx, sco.UnlockHash, txid)
					}
					for l, sco := range fc.MissedProofOutputs {
						scoid := fcid.StorageProofOutputID(types.ProofMissed, uint64(l))
						dbAddSiacoinOutputID(tx, scoid, txid)
						dbAddUnlockHash(tx, sco.UnlockHash, txid)
					}
				}
				for _, fcr := range txn.FileContractRevisions {
					dbAddFileContractID(tx, fcr.ParentID, txid)
					dbAddUnlockHash(tx, fcr.UnlockConditions.UnlockHash(), txid)
					dbAddUnlockHash(tx, fcr.NewUnlockHash, txid)
					for l, sco := range fcr.NewValidProofOutputs {
						scoid := fcr.ParentID.StorageProofOutputID(types.ProofValid, uint64(l))
						dbAddSiacoinOutputID(tx, scoid, txid)
						dbAddUnlockHash(tx, sco.UnlockHash, txid)
					}
					for l, sco := range fcr.NewMissedProofOutputs {
						scoid := fcr.ParentID.StorageProofOutputID(types.ProofMissed, uint64(l))
						dbAddSiacoinOutputID(tx, scoid, txid)
						dbAddUnlockHash(tx, sco.UnlockHash, txid)
					}
					dbAddFileContractRevision(tx, fcr.ParentID, fcr)
				}
				for _, sp := range txn.StorageProofs {
					dbAddFileContractID(tx, sp.ParentID, txid)
					dbAddStorageProof(tx, sp.ParentID, sp)
				}
				for _, sfi := range txn.SiafundInputs {
					dbAddSiafundOutputID(tx, sfi.ParentID, txid)
					dbAddUnlockHash(tx, sfi.UnlockConditions.UnlockHash(), txid)
					dbAddUnlockHash(tx, sfi.ClaimUnlockHash, txid)
				}
				for k, sfo := range txn.SiafundOutputs {
					sfoid := txn.SiafundOutputID(uint64(k))
					dbAddSiafundOutputID(tx, sfoid, txid)
					dbAddUnlockHash(tx, sfo.UnlockHash, txid)
				}
			}

			// calculate and add new block facts, if possible
			facts, err := e.dbCalculateBlockFacts(tx, e.cs, block)
			if err == nil {
				dbAddBlockFacts(tx, facts)
			} else {
				e.log.Printf("Error calculating block facts: %s", err)
				return err
			}
		}

		// Update stats according to SiacoinOutputDiffs
		for _, scod := range cc.SiacoinOutputDiffs {
			if scod.Direction == modules.DiffApply {
				dbAddSiacoinOutput(tx, scod.ID, scod.SiacoinOutput)
			}
		}

		// Update stats according to SiafundOutputDiffs
		for _, sfod := range cc.SiafundOutputDiffs {
			if sfod.Direction == modules.DiffApply {
				dbAddSiafundOutput(tx, sfod.ID, sfod.SiafundOutput)
			}
		}

		// Compute the changes in the active set. Note, because this is calculated
		// at the end instead of in a loop, the historic facts may contain
		// inaccuracies about the active set. This should not be a problem except
		// for large reorgs.
		// TODO: improve this
		currentBlock, exists := e.cs.BlockAtHeight(blockheight)
		if !exists && build.DEBUG {
			build.Critical("consensus is missing block", blockheight)
		} else if !exists {
			e.log.Printf("consensus is missing block: %s", blockheight)
			return
		}
		currentID := currentBlock.ID()
		var facts blockFacts

		err = dbGetAndDecode(bucketBlockFacts, currentID, &facts)(tx)
		if err == nil {
			for _, diff := range cc.FileContractDiffs {
				if diff.Direction == modules.DiffApply {
					facts.ActiveContractCount++
					facts.ActiveContractCost = facts.ActiveContractCost.Add(diff.FileContract.Payout)
					facts.ActiveContractSize = facts.ActiveContractSize.Add(types.NewCurrency64(diff.FileContract.FileSize))
				} else {
					facts.ActiveContractCount--
					facts.ActiveContractCost = facts.ActiveContractCost.Sub(diff.FileContract.Payout)
					facts.ActiveContractSize = facts.ActiveContractSize.Sub(types.NewCurrency64(diff.FileContract.FileSize))
				}
			}
			dbAddBlockFacts(tx, facts)
		} else {
			e.log.Printf("Error getting block facts for %s", currentBlock.ID())
			return err
		}

		e.log.Printf("Explorer update for block: %d", blockheight)
		e.persistMu.Lock()
		e.persist.Height = blockheight
		e.persist.RecentChange = cc.ID
		e.persist.Target = cc.ChildTarget
		e.persistMu.Unlock()

		err = e.saveSync()
		return err
	})

	if err != nil && build.DEBUG {
		build.Critical("explorer update failed:", err)
	} else if err != nil {
		e.log.Printf("explorer update failed: %s", err)
	}
}

func (e *Explorer) dbCalculateBlockFacts(tx *bolt.Tx, cs modules.ConsensusSet, block types.Block) (blockFacts, error) {
	// get the parent block facts
	var bf blockFacts
	err := dbGetAndDecode(bucketBlockFacts, block.ParentID, &bf)(tx)
	if err != nil {
		return bf, err
	}

	// get target
	target, exists := cs.ChildTarget(block.ParentID)
	if !exists && build.DEBUG {
		panic(fmt.Sprintf("ConsensusSet is missing target of known block: %s", block.ParentID))
	} else if !exists {
		e.log.Printf("ConsensusSet is missing target of known block: %s", block.ParentID)
	}

	// update fields
	bf.BlockID = block.ID()
	bf.Height++
	bf.Difficulty = target.Difficulty()
	bf.Target = target
	bf.Timestamp = block.Timestamp
	bf.TotalCoins = types.CalculateNumSiacoins(bf.Height)

	// calculate maturity timestamp
	var maturityTimestamp types.Timestamp
	if bf.Height > types.MaturityDelay {
		oldBlock, exists := cs.BlockAtHeight(bf.Height - types.MaturityDelay)
		if !exists && build.DEBUG {
			panic(fmt.Sprint("ConsensusSet is missing block at height", bf.Height-types.MaturityDelay))
		} else if !exists {
			e.log.Printf("ConsensusSet is missing block at height: %s", bf.Height-types.MaturityDelay)
		}
		maturityTimestamp = oldBlock.Timestamp
	}
	bf.MaturityTimestamp = maturityTimestamp

	// calculate hashrate by averaging last 'hashrateEstimationBlocks' blocks
	var estimatedHashrate types.Currency
	if bf.Height > hashrateEstimationBlocks {
		var totalDifficulty = bf.Target
		var oldestTimestamp types.Timestamp
		for i := types.BlockHeight(1); i < hashrateEstimationBlocks; i++ {
			b, exists := cs.BlockAtHeight(bf.Height - i)
			if !exists && build.DEBUG {
				panic(fmt.Sprintf("ConsensusSet is missing block at height: %s", bf.Height-hashrateEstimationBlocks))
			} else if !exists {
				e.log.Printf("ConsensusSet is missing block at height: %s", bf.Height-hashrateEstimationBlocks)
			}
			target, exists := cs.ChildTarget(b.ParentID)
			if !exists && build.DEBUG {
				panic(fmt.Sprintf("ConsensusSet is missing target of known block: %s", b.ParentID))
			} else if !exists {
				e.log.Printf("ConsensusSet is missing target of known block: %s", b.ParentID)
			}
			totalDifficulty = totalDifficulty.AddDifficulties(target)
			oldestTimestamp = b.Timestamp
		}
		secondsPassed := bf.Timestamp - oldestTimestamp
		estimatedHashrate = totalDifficulty.Difficulty().Div64(uint64(secondsPassed))
	}
	bf.EstimatedHashrate = estimatedHashrate

	bf.MinerPayoutCount += uint64(len(block.MinerPayouts))
	bf.TransactionCount += uint64(len(block.Transactions))
	for _, txn := range block.Transactions {
		bf.SiacoinInputCount += uint64(len(txn.SiacoinInputs))
		bf.SiacoinOutputCount += uint64(len(txn.SiacoinOutputs))
		bf.FileContractCount += uint64(len(txn.FileContracts))
		bf.FileContractRevisionCount += uint64(len(txn.FileContractRevisions))
		bf.StorageProofCount += uint64(len(txn.StorageProofs))
		bf.SiafundInputCount += uint64(len(txn.SiafundInputs))
		bf.SiafundOutputCount += uint64(len(txn.SiafundOutputs))
		bf.MinerFeeCount += uint64(len(txn.MinerFees))
		bf.ArbitraryDataCount += uint64(len(txn.ArbitraryData))
		bf.TransactionSignatureCount += uint64(len(txn.TransactionSignatures))

		for _, fc := range txn.FileContracts {
			bf.TotalContractCost = bf.TotalContractCost.Add(fc.Payout)
			bf.TotalContractSize = bf.TotalContractSize.Add(types.NewCurrency64(fc.FileSize))
		}
		for _, fcr := range txn.FileContractRevisions {
			bf.TotalContractSize = bf.TotalContractSize.Add(types.NewCurrency64(fcr.NewFileSize))
			bf.TotalRevisionVolume = bf.TotalRevisionVolume.Add(types.NewCurrency64(fcr.NewFileSize))
		}
	}

	return bf, nil
}

// Special handling for the genesis block. No other functions are called on it.
func dbAddGenesisBlock(tx *bolt.Tx) {
	id := types.GenesisID
	dbAddBlockID(tx, id, 0)
	txid := types.GenesisBlock.Transactions[0].ID()
	dbAddTransactionID(tx, txid, 0)
	for i, sfo := range types.GenesisSiafundAllocation {
		sfoid := types.GenesisBlock.Transactions[0].SiafundOutputID(uint64(i))
		dbAddSiafundOutputID(tx, sfoid, txid)
		dbAddUnlockHash(tx, sfo.UnlockHash, txid)
		dbAddSiafundOutput(tx, sfoid, sfo)
	}
	dbAddBlockFacts(tx, blockFacts{
		BlockFacts: modules.BlockFacts{
			BlockID:            id,
			Height:             0,
			Difficulty:         types.RootTarget.Difficulty(),
			Target:             types.RootTarget,
			TotalCoins:         types.CalculateCoinbase(0),
			TransactionCount:   1,
			SiafundOutputCount: uint64(len(types.GenesisSiafundAllocation)),
		},
		Timestamp: types.GenesisBlock.Timestamp,
	})
}
