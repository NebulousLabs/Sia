package blockexplorer

import (
	"errors"

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
	// Start the loop at i = 1 due to the genesis block
	for i := types.BlockHeight(1); i < height; i++ {
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
		return nil, errors.New("the start block must be higher than the end block")
	}
	if finish > be.blockchainHeight {
		return nil, errors.New("cannot get info on a block higher than the blockchain")
	}

	return be.blockSummaries[start:finish], nil
}

// Returns many pieces of readily available information
func (be *BlockExplorer) ExplorerStatus() modules.ExplorerStatus {
	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	// No reason that consensus should broadcast a block that it
	// doesn't have information on
	currentTarget, exists := be.cs.ChildTarget(be.currentBlock.ParentID)
	if build.DEBUG {
		if !exists {
			panic("The state of the current block cannot be found")
		}
	}

	return modules.ExplorerStatus{
		Height:              be.blockchainHeight,
		Block:               be.currentBlock,
		Target:              currentTarget,
		TotalCurrency:       totalCurrency(be.blockchainHeight),
		ActiveContractCount: be.activeContracts,
		ActiveContractCosts: be.activeContractCost,
		ActiveContractSize:  be.activeContractSize,
		TotalContractCount:  be.totalContracts,
		TotalContractCosts:  be.totalContractCost,
		TotalContractSize:   be.totalContractSize,
	}
}
