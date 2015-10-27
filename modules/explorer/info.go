package explorer

import (
	"errors"

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

// Returns a partial slice of our stored data on the blockchain. Data
// obtained from consensus updates
func (e *Explorer) BlockInfo(start types.BlockHeight, finish types.BlockHeight) ([]modules.ExplorerBlockData, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Error checking on the given range
	if start > finish {
		return nil, errors.New("the start block must be higher than the end block")
	}
	if finish > e.blockchainHeight+1 {
		return nil, errors.New("cannot get info on a block higher than the blockchain")
	}

	summaries, err := e.db.dbBlockSummaries(start, finish)
	if err != nil {
		return nil, err
	}
	return summaries, nil
}

// Returns many pieces of readily available information
func (e *Explorer) Statistics() modules.ExplorerStatistics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	target, _ := e.cs.ChildTarget(e.currentBlock)
	difficulty := types.NewCurrency(types.RootTarget.Int()).Div(types.NewCurrency(target.Int()))
	maturityTimestamp := e.seenTimes[(e.blockchainHeight-types.MaturityDelay)%types.BlockHeight(len(e.seenTimes))]
	return modules.ExplorerStatistics{
		Height:            e.blockchainHeight,
		Block:             e.currentBlock,
		Target:            target,
		Difficulty:        difficulty,
		MaturityTimestamp: types.Timestamp(maturityTimestamp.Unix()),
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
