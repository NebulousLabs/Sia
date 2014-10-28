package sia

import (
	"time"
)

// These values will be generated before release, but the code for generating
// them will never be released.  All that the rest of the world will see is
// hardcoded values.
func CreateGenesisBlock() (b *Block) {
	b = &Block{
		Version: 1,
		// Parent is 0.
		Timestamp: Timestamp(time.Now().Unix()),
		// Nonce is 0.
		// Miner Address is?
		// No transactions means 0 merkle root.
	}

	return
}

// Create the state that contains the genesis block and nothing else.
func CreateGenesisState() (s *State) {
	genesisBlock := CreateGenesisBlock()
	gbid := genesisBlock.ID()

	s = new(State)
	s.BadBlocks = make(map[BlockID]struct{})
	s.BlockMap = make(map[BlockID]*BlockNode)

	s.ConsensusState.UnspentOutputs = make(map[OutputID]Output)
	s.ConsensusState.SpentOutputs = make(map[OutputID]Output)

	s.BlockRoot = new(BlockNode)
	s.CurrentBlock = genesisBlock.ID()
	s.BlockMap[gbid] = s.BlockRoot

	return
}
