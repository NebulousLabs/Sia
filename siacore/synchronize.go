package siacore

import (
	"errors"

	"github.com/NebulousLabs/Andromeda/network"
)

const (
	MaxCatchUpBlocks = 100
)

var (
	GenesisAddress   = CoinAddress{}         // TODO: NEED TO CREATE A HARDCODED ADDRESS.
	GenesisTimestamp = Timestamp(1415904418) // Approx. 1:47pm EST Nov. 13th, 2014
)

// CreateGenesisState will create the state that contains the genesis block and
// nothing else.
func CreateGenesisState() (s *State) {
	// Create a new state and initialize the maps.
	s = new(State)
	s.BlockRoot = new(BlockNode)
	s.BadBlocks = make(map[BlockID]struct{})
	s.BlockMap = make(map[BlockID]*BlockNode)
	s.OrphanMap = make(map[BlockID]map[BlockID]*Block)
	s.CurrentPath = make(map[BlockHeight]BlockID)
	s.OpenContracts = make(map[ContractID]*OpenContract)
	s.UnspentOutputs = make(map[OutputID]Output)
	s.SpentOutputs = make(map[OutputID]Output)
	s.TransactionPool = make(map[OutputID]*Transaction)
	s.TransactionList = make(map[OutputID]*Transaction)

	// Create the genesis block and add it as the BlockRoot.
	genesisBlock := &Block{
		Timestamp:    GenesisTimestamp,
		MinerAddress: GenesisAddress,
	}
	s.BlockRoot.Block = genesisBlock
	s.BlockRoot.Height = 0
	for i := range s.BlockRoot.RecentTimestamps {
		s.BlockRoot.RecentTimestamps[i] = GenesisTimestamp
	}
	s.BlockRoot.Target[1] = 16 // Easy enough for a home computer to be able to mine on.
	s.BlockRoot.Depth[0] = 255 // depth of genesis block is set to 111111110000000000000000...
	s.BlockMap[genesisBlock.ID()] = s.BlockRoot

	// Fill out the consensus informaiton for the genesis block.
	s.CurrentBlockID = genesisBlock.ID()
	s.CurrentPath[BlockHeight(0)] = genesisBlock.ID()

	// Create the genesis subsidy output.
	genesisSubsidyOutput := Output{
		Value:     CalculateCoinbase(0),
		SpendHash: GenesisAddress,
	}
	s.UnspentOutputs[genesisBlock.SubsidyID()] = genesisSubsidyOutput

	return
}

// SendBlocks sends all known blocks from the given height forward from the
// longest known fork.
func (s *State) SendBlocks(knownBlocks [32]BlockID, blocks *[]Block) error {
	// Find the most recent block that is in our current path.
	found := false
	var closestHeight BlockHeight
	for i := range knownBlocks {
		// See if we know which block it is, get the node to know the height.
		blockNode, exists := s.BlockMap[knownBlocks[i]]
		if !exists {
			continue
		}

		// See if the known block is in the current path, and if it is see if
		// its height is greater than the closest yet known height.
		id, exists := s.CurrentPath[blockNode.Height]
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
	for i := closestHeight; i < closestHeight+MaxCatchUpBlocks; i++ {
		b := s.BlockAtHeight(i)
		if b == nil {
			break
		}
		*blocks = append(*blocks, *b)
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

		knownBlocks[i] = s.CurrentPath[s.Height()-i]
	}

	backtrace := BlockHeight(10)
	for i := 12; i < 31; i++ {
		backtrace = BlockHeight(float64(backtrace) * 1.75)
		// Prevent underflows
		if backtrace > s.Height() {
			break
		}

		knownBlocks[i] = s.CurrentPath[s.Height()-backtrace]
	}

	knownBlocks[31] = s.CurrentPath[0]

	var blocks []Block
	peer.RPC('R', knownBlocks, &blocks)

	for i := range blocks {
		if err = s.AcceptBlock(blocks[i]); err != nil {
			if err != BlockKnownErr && err != FutureBlockErr {
				// Return if there's an error, but don't return for benign
				// errors: BlockKnownErr and FutureBlockErr are both benign.
				return err
			}
		}
	}

	return nil
}
