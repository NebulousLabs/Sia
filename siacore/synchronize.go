package siacore

import (
	"errors"

	"github.com/NebulousLabs/Andromeda/network"
)

const (
	MaxCatchUpBlocks = 100
)

// CreateGenesisState will create the state that contains the genesis block and
// nothing else.
func CreateGenesisState() *State {
	// Create a new state and initialize the maps.
	s := &State{
		blockRoot:       new(BlockNode),
		badBlocks:       make(map[BlockID]struct{}),
		blockMap:        make(map[BlockID]*BlockNode),
		orphanMap:       make(map[BlockID]map[BlockID]*Block),
		currentPath:     make(map[BlockHeight]BlockID),
		openContracts:   make(map[ContractID]*OpenContract),
		unspentOutputs:  make(map[OutputID]Output),
		spentOutputs:    make(map[OutputID]Output),
		transactionPool: make(map[OutputID]*Transaction),
		transactionList: make(map[OutputID]*Transaction),
	}

	// Create the genesis block and add it as the BlockRoot.
	genesisBlock := &Block{
		Timestamp:    GenesisTimestamp,
		MinerAddress: GenesisAddress,
	}
	s.blockRoot.Block = genesisBlock
	s.blockRoot.Height = 0
	for i := range s.blockRoot.RecentTimestamps {
		s.blockRoot.RecentTimestamps[i] = GenesisTimestamp
	}
	s.blockRoot.Target[1] = 1  // Easy enough for a home computer to be able to mine on.
	s.blockRoot.Depth[0] = 255 // depth of genesis block is set to 111111110000000000000000...
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

	return s
}

// SendBlocks sends all known blocks from the given height forward from the
// longest known fork.
func (s *State) SendBlocks(knownBlocks [32]BlockID, blocks *[]Block) error {
	// Find the most recent block that is in our current path.
	found := false
	var closestHeight BlockHeight
	for i := range knownBlocks {
		// See if we know which block it is, get the node to know the height.
		blockNode, exists := s.blockMap[knownBlocks[i]]
		if !exists {
			continue
		}

		// See if the known block is in the current path, and if it is see if
		// its height is greater than the closest yet known height.
		id, exists := s.currentPath[blockNode.Height]
		if exists && id == knownBlocks[i] {
			found = true
			if closestHeight < blockNode.Height {
				closestHeight = blockNode.Height
			}
		}
	}

	// See that a match was actually found.
	if !found {
		return errors.New("no matching block found during SendBlocks")
	}

	// Build an array of blocks.
	tallest := closestHeight + MaxCatchUpBlocks
	if tallest > s.Height() {
		tallest = s.Height()
	}
	for i := closestHeight; i <= tallest; i++ {
		b, err := s.BlockAtHeight(i)
		if err != nil {
			panic(err)
		}
		*blocks = append(*blocks, b)
	}

	return nil
}

func (s *State) CatchUp(peer network.NetAddress) (err error) {
	var knownBlocks [32]BlockID
	for i := BlockHeight(0); i < 12; i++ {
		// Prevent underflows
		if i > s.Height() {
			break
		}

		knownBlocks[i] = s.currentPath[s.Height()-i]
	}

	backtrace := BlockHeight(10)
	for i := 12; i < 31; i++ {
		backtrace = BlockHeight(float64(backtrace) * 1.75)
		// Prevent underflows
		if backtrace > s.Height() {
			break
		}

		knownBlocks[i] = s.currentPath[s.Height()-backtrace]
	}

	knownBlocks[31] = s.currentPath[0]

	prevHeight := s.Height()

	var blocks []Block
	if err = peer.RPC("SendBlocks", knownBlocks, &blocks); err != nil {
		return err
	}

	for i := range blocks {
		if err = s.AcceptBlock(blocks[i]); err != nil {
			if err != BlockKnownErr && err != FutureBlockErr {
				// Return if there's an error, but don't return for benign
				// errors: BlockKnownErr and FutureBlockErr are both benign.
				return err
			}
		}
	}

	// recurse until the height stops increasing
	if prevHeight != s.Height() {
		return s.CatchUp(peer)
	}

	return nil
}
