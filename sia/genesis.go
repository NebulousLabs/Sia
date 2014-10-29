package sia

import (
	"time"
)

// These values will be generated before release, but the code for generating
// them will never be released.  All that the rest of the world will see is
// hardcoded values.
func CreateGenesisBlock(premineAddress CoinAddress) (b *Block) {
	b = &Block{
		Version: 1,
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
	// Create the genesis block using the premine address.
	genesisBlock := CreateGenesisBlock(premineAddress)
	gbid := genesisBlock.ID()

	// Create a new state and initialize the maps.
	s = new(State)
	s.BadBlocks = make(map[BlockID]struct{})
	s.BlockMap = make(map[BlockID]*BlockNode)

	// Initialize ConsensusState maps.
	s.ConsensusState.UnspentOutputs = make(map[OutputID]Output)
	s.ConsensusState.SpentOutputs = make(map[OutputID]Output)

	// Fill out the block root node, and add it to the BlockMap.
	s.BlockRoot = new(BlockNode)
	s.CurrentBlock = gbid
	s.BlockMap[gbid] = s.BlockRoot

	// Set the difficulty and timestamp information on the genesis block node.
	s.BlockRoot.Height = 0
	for i := range s.BlockRoot.RecentTimestamps {
		s.BlockRoot.RecentTimestamps[i] = Timestamp(time.Now().Unix())
	}
	s.BlockRoot.Difficulty[15] = 1
	s.BlockRoot.DifficultyReference = gbid // Points to genesis block until block DifficultyWindow + 1.

	return
}
