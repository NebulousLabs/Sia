package consensus

import (
	"errors"
	"fmt"
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
	blockRoot *BlockNode

	// Missing parents is a double map, the first a map of missing parents, and
	// the second is a map of the known children to the parent. The first is
	// necessary so that if a parent is found, all the children can be added to
	// the parent. The second is necessary for checking if a new block is a
	// known orphan.
	badBlocks      map[BlockID]struct{}          // A list of blocks that don't verify.
	blockMap       map[BlockID]*BlockNode        // A list of all blocks in the blocktree.
	missingParents map[BlockID]map[BlockID]Block // A list of all missing parents and their known children.

	// The transaction pool works by storing a list of outputs that are
	// spent by transactions in the pool, and pointing to the transaction
	// that spends them. That makes it really easy to look up conflicts as
	// new transacitons arrive, and also easy to remove transactions from
	// the pool (delete every input used in the transaction.) The
	// transaction list contains only the first output, so that when
	// building blocks you can more easily iterate through every
	// transaction.
	transactionPoolOutputs map[OutputID]*Transaction
	transactionPoolProofs  map[ContractID]*Transaction
	transactionList        map[OutputID]*Transaction

	// Consensus Variables - the current state of consensus according to the
	// longest fork.
	currentBlockID BlockID
	currentPath    map[BlockHeight]BlockID
	unspentOutputs map[OutputID]Output
	openContracts  map[ContractID]FileContract

	// TODO: docstring
	subscriptions []chan struct{}

	mu sync.RWMutex
}

// CreateGenesisState will create the state that contains the genesis block and
// nothing else.
func CreateGenesisState() (s *State) {
	// Create a new state and initialize the maps.
	s = &State{
		blockRoot:              new(BlockNode),
		badBlocks:              make(map[BlockID]struct{}),
		blockMap:               make(map[BlockID]*BlockNode),
		missingParents:         make(map[BlockID]map[BlockID]Block),
		currentPath:            make(map[BlockHeight]BlockID),
		openContracts:          make(map[ContractID]FileContract),
		unspentOutputs:         make(map[OutputID]Output),
		transactionPoolOutputs: make(map[OutputID]*Transaction),
		transactionPoolProofs:  make(map[ContractID]*Transaction),
		transactionList:        make(map[OutputID]*Transaction),
	}

	// Create the genesis block and add it as the BlockRoot.
	genesisBlock := Block{
		Timestamp:    GenesisTimestamp,
		MinerAddress: GenesisAddress,
	}
	s.blockRoot.Block = genesisBlock
	s.blockRoot.Height = 0
	s.blockRoot.Target = RootTarget
	s.blockRoot.Depth = RootDepth
	s.blockMap[genesisBlock.ID()] = s.blockRoot

	// Fill out the consensus informaiton for the genesis block.
	s.currentBlockID = genesisBlock.ID()
	s.currentPath[BlockHeight(0)] = genesisBlock.ID()

	// Create the genesis subsidy output.
	genesisSubsidyOutput := Output{
		Value:     CalculateCoinbase(0),
		SpendHash: GenesisAddress,
	}
	s.unspentOutputs[genesisBlock.SubsidyID()] = genesisSubsidyOutput

	return
}

// BlockAtHeight() returns the block from the current history at the
// input height.
func (s *State) blockAtHeight(height BlockHeight) (b Block, err error) {
	if bn, ok := s.blockMap[s.currentPath[height]]; ok {
		b = bn.Block
		return
	}
	err = fmt.Errorf("no block at height %v found.", height)
	return
}

// currentBlockNode returns the node of the most recent block in the
// longest fork.
func (s *State) currentBlockNode() *BlockNode {
	return s.blockMap[s.currentBlockID]
}

// CurrentBlockWeight() returns the weight of the current block in the
// heaviest fork.
func (s *State) currentBlockWeight() BlockWeight {
	return s.currentBlockNode().Target.Inverse()
}

// depth returns the depth of the current block of the state.
func (s *State) depth() Target {
	return s.currentBlockNode().Depth
}

// height returns the current height of the state.
func (s *State) height() BlockHeight {
	return s.blockMap[s.currentBlockID].Height
}

// State.Output returns the Output associated with the id provided for input,
// but only if the output is a part of the utxo set.
func (s *State) output(id OutputID) (output Output, exists bool) {
	output, exists = s.unspentOutputs[id]
	return
}

// Block returns the block associated with the given id.
func (s *State) Block(id BlockID) (b Block, exists bool) {
	node, exists := s.blockMap[id]
	if !exists {
		return
	}
	b = node.Block
	return
}

// BlockAtHeight returns the block in the current fork found at `height`.
func (s *State) BlockAtHeight(height BlockHeight) (b Block, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bn, exists := s.blockMap[s.currentPath[height]]
	if !exists {
		err = errors.New("no block found")
		return
	}
	b = bn.Block
	return
}

func (s *State) Contract(id ContractID) (fc FileContract, err error) {
	fc, exists := s.openContracts[id]
	if !exists {
		err = errors.New("no contract found")
		return
	}

	return
}

// CurrentBlock returns the highest block on the tallest fork.
func (s *State) CurrentBlock() Block {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentBlockNode().Block
}

// CurrentTarget returns the target of the next block that needs to be
// submitted to the state.
func (s *State) CurrentTarget() Target {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentBlockNode().Target
}

// EarliestLegalTimestamp returns the earliest legal timestamp of the next
// block - earlier timestamps will render the block invalid.
func (s *State) EarliestTimestamp() Timestamp {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentBlockNode().earliestChildTimestamp()
}

// State.Height() returns the height of the longest fork.
func (s *State) Height() BlockHeight {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.height()
}

// HeightOfBlock returns the height of the block with id `bid`.
func (s *State) HeightOfBlock(bid BlockID) (height BlockHeight, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bn, exists := s.blockMap[bid]
	if !exists {
		err = errors.New("block not found")
		return
	}
	height = bn.Height
	return
}

func (s *State) Output(id OutputID) (output Output, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.output(id)
}

func (s *State) RLock() {
	s.mu.RLock()
}

func (s *State) RUnlock() {
	s.mu.RUnlock()
}
