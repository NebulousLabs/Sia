package sia

import (
	"math/big"
	"time"
)

// These values will be generated before release, but the code for generating
// them will never be released.  All that the rest of the world will see is
// hardcoded values.
func CreateGenesisBlock(premineAddress CoinAddress) (b *Block) {
	b = &Block{
		// Parent is 0.
		Timestamp: Timestamp(time.Now().Unix()),
		// Nonce is 0.
		MinerAddress: premineAddress,
		// No transactions means 0 merkle root.
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
	s.ConsensusState.UnspentOutputs = make(map[OutputID]Output)
	s.ConsensusState.SpentOutputs = make(map[OutputID]Output)

	// Create the genesis block using the premine address.
	genesisBlock := CreateGenesisBlock(premineAddress)

	// Fill out the block root node, and add it to the BlockMap.
	s.BlockRoot.Block = genesisBlock
	s.BlockRoot.Height = 0
	for i := range s.BlockRoot.RecentTimestamps {
		s.BlockRoot.RecentTimestamps[i] = Timestamp(time.Now().Unix())
	}
	s.BlockRoot.Target[1] = 1 // LOL my laptop gets like 10 khash/s
	s.BlockRoot.Depth = big.NewRat(0, 1)
	s.BlockMap[genesisBlock.ID()] = s.BlockRoot

	// Fill out the ConsensusState
	s.ConsensusState.CurrentBlock = genesisBlock.ID()
	s.ConsensusState.CurrentPath[BlockHeight(0)] = genesisBlock.ID()

	return
}
