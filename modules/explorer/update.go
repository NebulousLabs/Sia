package explorer

import (
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// ProcessConsensusChange follows the most recent changes to the consensus set,
// including parsing new blocks and updating the utxo sets.
func (e *Explorer) ProcessConsensusChange(cc modules.ConsensusChange) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Add the stats for the reverted blocks.
	for _, block := range cc.RevertedBlocks {
		for _, txn := range block.Transactions {
			e.transactionCount--
			for _ = range txn.SiacoinInputs {
				e.siacoinInputCount--
			}
			for _ = range txn.SiacoinOutputs {
				e.siacoinOutputCount--
			}
			for _, fc := range txn.FileContracts {
				e.fileContractCount--
				e.totalContractCost = e.totalContractCost.Sub(fc.Payout)
				e.totalContractSize = e.totalContractSize.Sub(types.NewCurrency64(fc.FileSize))
			}
			for _, fcr := range txn.FileContractRevisions {
				e.fileContractRevisionCount--
				e.totalContractSize = e.totalContractSize.Sub(types.NewCurrency64(fcr.NewFileSize))
				e.totalRevisionVolume = e.totalRevisionVolume.Sub(types.NewCurrency64(fcr.NewFileSize))
			}
			for _ = range txn.StorageProofs {
				e.storageProofCount--
			}
			for _ = range txn.SiafundInputs {
				e.siafundInputCount--
			}
			for _ = range txn.SiafundOutputs {
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

	// Add the stats for the applied blocks.
	for _, block := range cc.AppliedBlocks {
		for _, txn := range block.Transactions {
			e.transactionCount++
			for _ = range txn.SiacoinInputs {
				e.siacoinInputCount++
			}
			for _ = range txn.SiacoinOutputs {
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

	// Reverting the blockheight and block data structs from reverted blocks
	e.blockchainHeight -= types.BlockHeight(len(cc.RevertedBlocks))

	// Handle incoming blocks
	for _, block := range cc.AppliedBlocks {
		e.blockchainHeight += 1

		// Add the current time to seenTimes
		if time.Unix(int64(block.Timestamp), 0).Before(e.startTime) {
			e.seenTimes[e.blockchainHeight%types.BlockHeight(len(e.seenTimes))] = time.Unix(int64(block.Timestamp), 0)
		} else {
			e.seenTimes[e.blockchainHeight%types.BlockHeight(len(e.seenTimes))] = time.Now()
		}
	}
	e.currentBlock = cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID()
}
