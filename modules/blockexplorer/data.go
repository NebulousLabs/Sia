package blockexplorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

func (es *ExplorerState) totalCurrency() types.Currency {
	totalCoins := uint64(0)
	coinbase := uint64(300e3)
	minCoinbase := uint64(30e3)
	for i := types.BlockHeight(0); i < es.blockchainHeight; i++ {
		totalCoins += coinbase
		if coinbase > minCoinbase {
			coinbase--
		}
	}
	return types.NewCurrency64(totalCoins).Mul(types.NewCurrency(types.CoinbaseAugment))
}

// Returns a partial slice of our stored data on the blockchain
func (es *ExplorerState) BlockInfo(start types.BlockHeight, finish types.BlockHeight) ([]modules.BlockData, error) {
	lockID := es.mu.RLock()
	defer es.mu.RUnlock(lockID)

	// Error checking on the given range
	if start > finish {
		return nil, errors.New("The start block must be higher than the end block")
	}
	if finish > es.blockchainHeight {
		return nil, errors.New("Cannot get info on a block higher than the blockchain")
	}

	return es.blocks[start:finish], nil
}

func (es *ExplorerState) BlockHeight() types.BlockHeight {
	lockID := es.mu.RLock()
	defer es.mu.RUnlock(lockID)

	return es.blockchainHeight
}

func (es *ExplorerState) CurrentBlock() modules.CurrentBlockData {
	lockID := es.mu.RLock()
	defer es.mu.RUnlock(lockID)

	// Taken straight from api/consensus.go
	currentTarget, exists := es.cs.ChildTarget(es.currentBlock.ID())
	if build.DEBUG {
		if !exists {
			panic("The state of the current block cannot be found")
		}
	}

	return modules.CurrentBlockData{
		Block:  es.currentBlock,
		Target: currentTarget,
	}
}

func (es *ExplorerState) SiaCoins() modules.SiacoinData {
	lockID := es.mu.RLock()
	defer es.mu.RUnlock(lockID)

	return modules.SiacoinData{
		CurrencySent:  es.currencySent,
		TotalCurrency: es.totalCurrency(),
	}
}

func (es *ExplorerState) FileContracts() modules.FileContractData {
	lockID := es.mu.RLock()
	defer es.mu.RUnlock(lockID)

	return modules.FileContractData{
		FileContractCount: es.fileContracts,
		FileContractCosts: es.fileContractCost,
	}
}
