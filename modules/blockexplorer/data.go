package blockexplorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

func (be *BlockExplorer) totalCurrency() types.Currency {
	totalCoins := uint64(0)
	coinbase := uint64(300e3)
	minCoinbase := uint64(30e3)
	for i := types.BlockHeight(0); i < be.blockchainHeight; i++ {
		totalCoins += coinbase
		if coinbase > minCoinbase {
			coinbase--
		}
	}
	return types.NewCurrency64(totalCoins).Mul(types.NewCurrency(types.CoinbaseAugment))
}

// Returns a partial slice of our stored data on the blockchain
func (be *BlockExplorer) BlockInfo(start types.BlockHeight, finish types.BlockHeight) ([]modules.ExplorerBlockData, error) {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	// Error checking on the given range
	if start > finish {
		return nil, errors.New("The start block must be higher than the end block")
	}
	if finish > be.blockchainHeight+1 {
		return nil, errors.New("Cannot get info on a block higher than the blockchain")
	}

	return be.blocks[start:finish], nil
}

func (be *BlockExplorer) BlockHeight() types.BlockHeight {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	return be.blockchainHeight
}

func (be *BlockExplorer) CurrentBlock() modules.ExplorerCurrentBlockData {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	// Taken straight from api/consensus.go
	currentTarget, exists := be.cs.ChildTarget(be.currentBlock.ID())
	if build.DEBUG {
		if !exists {
			panic("The state of the current block cannot be found")
		}
	}

	return modules.ExplorerCurrentBlockData{
		Block:  be.currentBlock,
		Target: currentTarget,
	}
}

func (be *BlockExplorer) Siacoins() modules.ExplorerSiacoinData {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	return modules.ExplorerSiacoinData{
		CurrencySent:  be.currencySent,
		TotalCurrency: be.totalCurrency(),
	}
}

func (be *BlockExplorer) FileContracts() modules.ExplorerFileContractData {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	return modules.ExplorerFileContractData{
		FileContractCount: be.fileContracts,
		FileContractCosts: be.fileContractCost,
	}
}
