package sia

import (
	"math/big"
	"time"
)

const (
	GenesisSubsidy = Currency(25000)
)

// These values will be generated before release, but the code for generating
// them will never be released.  All that the rest of the world will see is
// hardcoded values.
func CreateGenesisBlock(premineAddress CoinAddress) (b *Block) {
	b = &Block{
		Timestamp:    Timestamp(time.Now().Unix()),
		MinerAddress: premineAddress,
	}

	return
}

// Create the state that contains the genesis block and nothing else.
func CreateGenesisState(premineAddress CoinAddress) (s *State) {
	// Create a new state and initialize the maps.
	s = new(State)
	s.BlockRoot = new(BlockNode)
	s.BadBlocks = make(map[BlockID]struct{})
	s.BlockMap = make(map[BlockID]*BlockNode)

	// Initialize ConsensusState maps.
	s.ConsensusState.CurrentPath = make(map[BlockHeight]BlockID)
	s.ConsensusState.OpenContracts = make(map[ContractID]*OpenContract)
	s.ConsensusState.UnspentOutputs = make(map[OutputID]Output)
	s.ConsensusState.SpentOutputs = make(map[OutputID]Output)
	s.ConsensusState.TransactionPool = make(map[OutputID]*Transaction)
	s.ConsensusState.TransactionList = make(map[OutputID]*Transaction)

	// Create the genesis block using the premine address.
	genesisBlock := CreateGenesisBlock(premineAddress)

	// Fill out the block root node, and add it to the BlockMap.
	s.BlockRoot.Block = genesisBlock
	s.BlockRoot.Height = 0
	for i := range s.BlockRoot.RecentTimestamps {
		s.BlockRoot.RecentTimestamps[i] = Timestamp(time.Now().Unix())
	}
	s.BlockRoot.Target[1] = 16
	s.BlockRoot.Depth = big.NewRat(0, 1)
	s.BlockMap[genesisBlock.ID()] = s.BlockRoot

	// Fill out the ConsensusState
	s.ConsensusState.CurrentBlock = genesisBlock.ID()
	s.ConsensusState.CurrentPath[BlockHeight(0)] = genesisBlock.ID()

	// Create the genesis subsidy output.
	genesisSubsidyOutput := Output{
		Value:     GenesisSubsidy,
		SpendHash: premineAddress,
	}
	s.ConsensusState.UnspentOutputs[genesisBlock.subsidyID()] = genesisSubsidyOutput

	return
}
