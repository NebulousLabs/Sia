package explorer

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
func (e *Explorer) ExplorerStatus() modules.ExplorerStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// No reason that consensus should broadcast a block that it
	// doesn't have information on
	var currentTarget types.Target
	if e.currentBlock.ID() == e.genesisBlockID {
		currentTarget = types.RootDepth
	} else {
		var exists bool
		currentTarget, exists = e.cs.ChildTarget(e.currentBlock.ParentID)
		if build.DEBUG {
			if !exists {
				panic("The state of the current block cannot be found")
			}
		}
	}

	// Find the seen time of the block 144 ago in the list
	matureBlockTime := e.seenTimes[(e.blockchainHeight-144)%types.BlockHeight(len(e.seenTimes))]

	return modules.ExplorerStatus{
		Height:              e.blockchainHeight,
		Block:               e.currentBlock,
		Target:              currentTarget,
		MatureTime:          types.Timestamp(matureBlockTime.Unix()),
		TotalCurrency:       totalCurrency(e.blockchainHeight),
		ActiveContractCount: e.activeContractCount,
		ActiveContractCosts: e.activeContractCost,
		ActiveContractSize:  e.activeContractSize,
		TotalContractCount:  e.totalContractCount,
		TotalContractCosts:  e.totalContractCost,
		TotalContractSize:   e.totalContractSize,
	}
}
