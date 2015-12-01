package explorer

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Block takes a block id and finds the corresponding block, provided that the
// block is in the consensus set.
func (e *Explorer) Block(id types.BlockID) (types.Block, types.BlockHeight, bool) {
	height, exists := e.blockHashes[id]
	if !exists {
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
	if height >= types.BlockHeight(len(e.historicFacts)) {
		return modules.BlockFacts{}, false
	}

	bf := e.historicFacts[height]
	return modules.BlockFacts{
		BlockID: bf.currentBlock,
		Height:  bf.blockchainHeight,

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

// Transaction takes a transaction id and finds the block containing the
// transaction. Because of the miner payouts, the transaction id might be a
// block id. To find the transaction, iterate through the block.
func (e *Explorer) Transaction(id types.TransactionID) (types.Block, types.BlockHeight, bool) {
	height, exists := e.transactionHashes[id]
	if !exists {
		return types.Block{}, 0, false
	}
	block, exists := e.cs.BlockAtHeight(height)
	if !exists {
		return types.Block{}, 0, false
	}
	return block, height, true
}

// UnlockHash returns the ids of all the transactions that contain the unlock
// hash. An empty set indicates that the unlock hash does not appear in the
// blockchain.
func (e *Explorer) UnlockHash(uh types.UnlockHash) []types.TransactionID {
	txnMap, exists := e.unlockHashes[uh]
	if !exists || len(txnMap) == 0 {
		return nil
	}
	ids := make([]types.TransactionID, 0, len(txnMap))
	for txid := range txnMap {
		ids = append(ids, txid)
	}
	return ids
}

// SiacoinOutputID returns all of the transactions that contain the input
// siacoin output id. An empty set indicates that the siacoin output id does
// not appear in the blockchain.
func (e *Explorer) SiacoinOutputID(id types.SiacoinOutputID) []types.TransactionID {
	txnMap, exists := e.siacoinOutputIDs[id]
	if !exists || len(txnMap) == 0 {
		return nil
	}
	ids := make([]types.TransactionID, 0, len(txnMap))
	for txid := range txnMap {
		ids = append(ids, txid)
	}
	return ids
}

// FileContractIDs returns all of the transactions that contain the input file
// contract id. An empty set indicates that the file contract id does not
// appear in the blockchain.
func (e *Explorer) FileContractID(id types.FileContractID) []types.TransactionID {
	txnMap, exists := e.fileContractIDs[id]
	if !exists || len(txnMap) == 0 {
		return nil
	}
	ids := make([]types.TransactionID, 0, len(txnMap))
	for txid := range txnMap {
		ids = append(ids, txid)
	}
	return ids
}

// SiafundOutputID returns all of the transactions that contain the input
// siafund output id. An empty set indicates that the siafund output id does
// not appear in the blockchain.
func (e *Explorer) SiafundOutputID(id types.SiafundOutputID) []types.TransactionID {
	txnMap, exists := e.siafundOutputIDs[id]
	if !exists || len(txnMap) == 0 {
		return nil
	}
	ids := make([]types.TransactionID, 0, len(txnMap))
	for txid := range txnMap {
		ids = append(ids, txid)
	}
	return ids
}
