package explorer

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Calculates the total number of coins that have ever been created
// from the bockheight
func totalCurrency(height types.BlockHeight) types.Currency {
	totalCoins := uint64(0)
	coinbase := types.InitialCoinbase
	minCoinbase := types.MinimumCoinbase
	for i := types.BlockHeight(0); i <= height; i++ {
		totalCoins += coinbase
		if coinbase > minCoinbase {
			coinbase--
		}
	}
	return types.NewCurrency64(totalCoins).Mul(types.SiacoinPrecision)
}

// Returns many pieces of readily available information
func (e *Explorer) Statistics() modules.ExplorerStatistics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	target, _ := e.cs.ChildTarget(e.currentBlock)
	difficulty := types.NewCurrency(types.RootTarget.Int()).Div(types.NewCurrency(target.Int()))
	currentBlock, exists := e.cs.BlockAtHeight(e.blockchainHeight)
	if build.DEBUG && !exists {
		panic("current block not found in consensus set")
	}
	return modules.ExplorerStatistics{
		Height:            e.blockchainHeight,
		CurrentBlock:      e.currentBlock,
		Target:            target,
		Difficulty:        difficulty,
		MaturityTimestamp: currentBlock.Timestamp,
		Circulation:       totalCurrency(e.blockchainHeight),

		TransactionCount:          e.transactionCount,
		SiacoinInputCount:         e.siacoinInputCount,
		SiacoinOutputCount:        e.siacoinOutputCount,
		FileContractCount:         e.fileContractCount,
		FileContractRevisionCount: e.fileContractRevisionCount,
		StorageProofCount:         e.storageProofCount,
		SiafundInputCount:         e.siafundInputCount,
		SiafundOutputCount:        e.siafundOutputCount,
		MinerFeeCount:             e.minerFeeCount,
		ArbitraryDataCount:        e.arbitraryDataCount,
		TransactionSignatureCount: e.transactionSignatureCount,

		ActiveContractCount: e.activeContractCount,
		ActiveContractCost:  e.activeContractCost,
		ActiveContractSize:  e.activeContractSize,
		TotalContractCost:   e.totalContractCost,
		TotalContractSize:   e.totalContractSize,
	}
}

// Block takes a block id and finds the corresponding block, provided that the
// block is in the consensus set.
func (e *Explorer) Block(id types.BlockID) (types.Block, bool) {
	height, exists := e.blockHashes[id]
	if !exists {
		return types.Block{}, false
	}
	return e.cs.BlockAtHeight(height)
}

// Transaction takes a transaction id and finds the block containing the
// transaction. Because of the miner payouts, the transaction id might be a
// block id. To find the transaction, iterate through the block.
func (e *Explorer) Transaction(id types.TransactionID) (types.Block, bool) {
	height, exists := e.transactionHashes[id]
	if !exists {
		return types.Block{}, false
	}
	return e.cs.BlockAtHeight(height)
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
