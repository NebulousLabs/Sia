package blockexplorer

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Calculates the total number of coins that have ever been created
// from the bockheight
func (be *BlockExplorer) totalCurrency() types.Currency {
	totalCoins := uint64(0)
	coinbase := types.InitialCoinbase
	minCoinbase := types.MinimumCoinbase
	for i := types.BlockHeight(0); i < be.blockchainHeight; i++ {
		totalCoins += coinbase
		if coinbase > minCoinbase {
			coinbase--
		}
	}
	return types.NewCurrency64(totalCoins).Mul(types.SiacoinPrecision)
}

// Returns a partial slice of our stored data on the blockchain. Data
// obtained from consensus updates
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

// Returns a data structure with the current block, and its child target
func (be *BlockExplorer) CurrentBlock() modules.ExplorerCurrentBlockData {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	// No reason that consensus should broadcast a block that it
	// doesn't have information on
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

// Returns a struct containing the total currency in the system and
// amount which has been part of a transaction
func (be *BlockExplorer) Siacoins() modules.ExplorerSiacoinData {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	return modules.ExplorerSiacoinData{
		CurrencySent:  be.currencySent,
		TotalCurrency: be.totalCurrency(),
	}
}

// Returns a struct containing the number of file contracts in the
// blockchain so far and how much they costed
func (be *BlockExplorer) FileContracts() modules.ExplorerFileContractData {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	return modules.ExplorerFileContractData{
		FileContractCount: be.fileContracts,
		FileContractCosts: be.fileContractCost,
	}
}
