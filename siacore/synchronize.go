package siacore

import (
	"errors"
	"net"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/network"
)

var (
	GenesisAddress   = CoinAddress{}         // NEED TO CREATE A HARDCODED ADDRESS.
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
func (s *State) SendBlocks(conn net.Conn, data []byte) (err error) {
	// Get the starting point.
	start := BlockHeight(encoding.DecUint64(data))
	end := s.Height()
	if start > end {
		err = errors.New("start is greater than the height of the longest known fork.")
		return
	}

	// Build an array of blocks.
	blocks := make([]Block, end-start+1)
	for i := range blocks {
		b := s.BlockAtHeight(start + BlockHeight(i))
		if b == nil {
			panic("nil block in state!")
		}
		blocks[i] = *b
	}

	// Encode and send the blocks.
	encBlocks := encoding.Marshal(blocks)
	encLen := encoding.EncUint64(uint64(len(encBlocks)))
	_, err = conn.Write(append(encLen[:4], encBlocks...))
	if err != nil {
		return
	}

	return
}

// catchUp handles orphan blocks and situations where the node has fallen
// behind the longest fork.
//
// NOTE: CATCHUP IS BROKEN FOR ANY VALUES OTHER THAN 1.
// NOTE: CATCHUP MIGHT SEND A SINGLE MESSAGE ASKING FOR MANY MEGABYTES WORTH OF BLOCKS.
func (s *State) CatchUp(start BlockHeight) func(net.Conn) error {
	encbh := encoding.EncUint64(uint64(start))
	return func(conn net.Conn) error {
		conn.Write(append([]byte{'R', 4, 0, 0, 0}, encbh[:4]...))
		var blocks []Block
		encBlocks, err := network.ReadPrefix(conn)
		if err != nil {
			return err
		}
		if err = encoding.Unmarshal(encBlocks, &blocks); err != nil {
			return err
		}
		for i := range blocks {
			if err = s.AcceptBlock(blocks[i]); err != nil {
				return err
			}
		}
		return nil
	}
}
