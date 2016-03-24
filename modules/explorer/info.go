package explorer

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Block takes a block ID and finds the corresponding block, provided that the
// block is in the consensus set.
func (e *Explorer) Block(id types.BlockID) (types.Block, types.BlockHeight, bool) {
	var height types.BlockHeight
	err := e.db.View(dbGetBlockHeight(id, &height))
	if err != nil {
		return types.Block{}, 0, false
	}
	block, exists := e.cs.BlockAtHeight(height)
	if !exists {
		return types.Block{}, 0, false
	}
	return block, height, true
}

// BlockFacts returns a set of statistics about the blockchain as they appeared
// at a given block height, and a bool indicating whether facts exist for the
// given height.
func (e *Explorer) BlockFacts(height types.BlockHeight) (modules.BlockFacts, bool) {
	block, exists := e.cs.BlockAtHeight(height)
	if !exists {
		return modules.BlockFacts{}, false
	}

	var bf blockFacts
	err := e.db.View(dbGetBlockFacts(block.ID(), &bf))
	if err != nil {
		return modules.BlockFacts{}, false
	}

	// convert to modules.BlockFacts
	return modules.BlockFacts{
		BlockID:           bf.currentBlock,
		Difficulty:        bf.target.Difficulty(),
		EstimatedHashrate: bf.estimatedHashrate,
		Height:            bf.blockchainHeight,
		MaturityTimestamp: bf.maturityTimestamp,
		Target:            bf.target,
		TotalCoins:        bf.totalCoins,

		// Transaction type counts.
		MinerPayoutCount:          bf.minerPayoutCount,
		TransactionCount:          bf.transactionCount,
		SiacoinInputCount:         bf.siacoinInputCount,
		SiacoinOutputCount:        bf.siacoinOutputCount,
		FileContractCount:         bf.fileContractCount,
		FileContractRevisionCount: bf.fileContractRevisionCount,
		StorageProofCount:         bf.storageProofCount,
		SiafundInputCount:         bf.siafundInputCount,
		SiafundOutputCount:        bf.siafundOutputCount,
		MinerFeeCount:             bf.minerFeeCount,
		ArbitraryDataCount:        bf.arbitraryDataCount,
		TransactionSignatureCount: bf.transactionSignatureCount,

		// Factoids about file contracts.
		ActiveContractCost:  bf.activeContractCost,
		ActiveContractCount: bf.activeContractCount,
		ActiveContractSize:  bf.activeContractSize,
		TotalContractCost:   bf.totalContractCost,
		TotalContractSize:   bf.totalContractSize,
		TotalRevisionVolume: bf.totalRevisionVolume,
	}, true
}

// Transaction takes a transaction ID and finds the block containing the
// transaction. Because of the miner payouts, the transaction ID might be a
// block ID. To find the transaction, iterate through the block.
func (e *Explorer) Transaction(id types.TransactionID) (types.Block, types.BlockHeight, bool) {
	var height types.BlockHeight
	err := e.db.View(dbGetTransactionHeight(id, &height))
	if err != nil {
		return types.Block{}, 0, false
	}
	block, exists := e.cs.BlockAtHeight(height)
	if !exists {
		return types.Block{}, 0, false
	}
	return block, height, true
}

// UnlockHash returns the IDs of all the transactions that contain the unlock
// hash. An empty set indicates that the unlock hash does not appear in the
// blockchain.
func (e *Explorer) UnlockHash(uh types.UnlockHash) []types.TransactionID {
	var ids []types.TransactionID
	err := e.db.View(dbGetUnlockHashTxnIDs(uh, &ids))
	if err != nil {
		ids = nil
	}
	return ids
}

// SiacoinOutput returns the siacoin output associated with the specified ID.
func (e *Explorer) SiacoinOutput(id types.SiacoinOutputID) (types.SiacoinOutput, bool) {
	var sco types.SiacoinOutput
	err := e.db.View(dbGetSiacoinOutput(id, &sco))
	if err != nil {
		return types.SiacoinOutput{}, false
	}
	return sco, true
}

// SiacoinOutputID returns all of the transactions that contain the specified
// siacoin output ID. An empty set indicates that the siacoin output ID does
// not appear in the blockchain.
func (e *Explorer) SiacoinOutputID(id types.SiacoinOutputID) []types.TransactionID {
	var ids []types.TransactionID
	err := e.db.View(dbGetSiacoinOutputTxnIDs(id, &ids))
	if err != nil {
		ids = nil
	}
	return ids
}

// FileContractHistory returns the history associated with the specified file
// contract ID, which includes the file contract itself and all of the
// revisions that have been submitted to the blockchain. The first bool
// indicates whether the file contract exists, and the second bool indicates
// whether a storage proof was successfully submitted for the file contract.
func (e *Explorer) FileContractHistory(id types.FileContractID) (fc types.FileContract, fcrs []types.FileContractRevision, fcE bool, spE bool) {
	var history fileContractHistory
	err := e.db.View(dbGetFileContractHistory(id, &history))
	fc = history.contract
	fcrs = history.revisions
	fcE = err == nil
	spE = history.storageProof.ParentID == id
	return
}

// FileContractIDs returns all of the transactions that contain the specified
// file contract ID. An empty set indicates that the file contract ID does not
// appear in the blockchain.
func (e *Explorer) FileContractID(id types.FileContractID) []types.TransactionID {
	var ids []types.TransactionID
	err := e.db.View(dbGetFileContractTxnIDs(id, &ids))
	if err != nil {
		ids = nil
	}
	return ids
}

// SiafundOutput returns the siafund output associated with the specified ID.
func (e *Explorer) SiafundOutput(id types.SiafundOutputID) (types.SiafundOutput, bool) {
	var sco types.SiafundOutput
	err := e.db.View(dbGetSiafundOutput(id, &sco))
	if err != nil {
		return types.SiafundOutput{}, false
	}
	return sco, true
}

// SiafundOutputID returns all of the transactions that contain the specified
// siafund output ID. An empty set indicates that the siafund output ID does
// not appear in the blockchain.
func (e *Explorer) SiafundOutputID(id types.SiafundOutputID) []types.TransactionID {
	var ids []types.TransactionID
	err := e.db.View(dbGetSiafundOutputTxnIDs(id, &ids))
	if err != nil {
		ids = nil
	}
	return ids
}
