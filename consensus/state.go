package consensus

import (
	"math/big"
	"sync"
)

type (
	BlockWeight *big.Rat
)

// Contains basic information about the state, but does not go into depth.
type StateInfo struct {
	CurrentBlock BlockID
	Height       BlockHeight
	Target       Target
}

type State struct {
	// The block root operates like a linked list of blocks, forming the
	// blocktree.
	blockRoot *blockNode

	// TODO: explain bad blocks.
	//
	// Missing parents is a double map, the first a map of missing parents, and
	// the second is a map of the known children to the parent. The first is
	// necessary so that if a parent is found, all the children can be added to
	// the parent. The second is necessary for checking if a new block is a
	// known orphan.
	badBlocks map[BlockID]struct{}   // A list of blocks that don't verify.
	blockMap  map[BlockID]*blockNode // A list of all blocks in the blocktree.

	// Consensus Variables - the current state of consensus according to the
	// longest fork.
	currentBlockID        BlockID
	currentPath           map[BlockHeight]BlockID
	siafundPool           Currency
	unspentSiafundOutputs map[OutputID]SiafundOutput
	unspentOutputs        map[OutputID]Output
	openContracts         map[ContractID]FileContract

	mu sync.RWMutex
}

// CreateGenesisState will create the state that contains the genesis block and
// nothing else. genesisTime is taken as an input instead of the constant being
// used directly because it makes certain parts of testing a lot easier.
func CreateGenesisState(genesisTime Timestamp) (s *State) {
	// Create a new state and initialize the maps.
	s = &State{
		badBlocks:             make(map[BlockID]struct{}),
		blockMap:              make(map[BlockID]*blockNode),
		currentPath:           make(map[BlockHeight]BlockID),
		openContracts:         make(map[ContractID]FileContract),
		unspentOutputs:        make(map[OutputID]Output),
		unspentSiafundOutputs: make(map[OutputID]SiafundOutput),
	}

	// Create the genesis block and add it as the BlockRoot.
	genesisBlock := Block{
		Timestamp: genesisTime,
	}
	s.blockRoot = &blockNode{
		block:  genesisBlock,
		target: RootTarget,
		depth:  RootDepth,
	}
	s.blockMap[genesisBlock.ID()] = s.blockRoot

	// Fill out the consensus informaiton for the genesis block.
	s.currentBlockID = genesisBlock.ID()
	s.currentPath[BlockHeight(0)] = genesisBlock.ID()
	s.unspentOutputs[genesisBlock.MinerPayoutID(0)] = Output{
		Value:     CalculateCoinbase(0),
		SpendHash: ZeroAddress,
	}
	s.unspentSiafundOutputs[OutputID{0}] = SiafundOutput{
		Value:            NewCurrency64(SiafundCount),
		SpendHash:        ZeroAddress, // TODO: change to Nebulous Genesis Address
		ClaimDestination: ZeroAddress, // TODO: change to Nebulous Genesis Address
	}

	return
}

func (s *State) RLock() {
	s.mu.RLock()
}

func (s *State) RUnlock() {
	s.mu.RUnlock()
}
