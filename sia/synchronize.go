package sia

import (
	"errors"
	"net"
	"time"
)

var (
	GenesisAddress   = CoinAddress{} // NEED TO CREATE A HARDCODED ADDRESS.
	GenesisTimestamp = 1415904418    // Approx. 1:47pm EST Nov. 13th, 2014
)

// Create the state that contains the genesis block and nothing else.
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
		Timestamp:    Timestamp(time.Now().Unix()),
		MinerAddress: GenesisAddress,
	}
	s.BlockRoot.Block = genesisBlock
	s.BlockRoot.Height = 0
	for i := range s.BlockRoot.RecentTimestamps {
		s.BlockRoot.RecentTimestamps[i] = Timestamp(time.Now().Unix())
	}
	s.BlockRoot.Target[1] = 16 // Easy enough for a home computer to be able to mine on.
	s.BlockRoot.Depth[0] = 255 // depth of genesis block is set to 111111110000000000000000...
	s.BlockMap[genesisBlock.ID()] = s.BlockRoot

	// Fill out the consensus informaiton for the genesis block.
	s.CurrentBlock = genesisBlock.ID()
	s.CurrentPath[BlockHeight(0)] = genesisBlock.ID()

	// Create the genesis subsidy output.
	genesisSubsidyOutput := Output{
		Value:     CalculateCoinbase(0),
		SpendHash: GenesisAddress,
	}
	s.UnspentOutputs[genesisBlock.SubsidyID()] = genesisSubsidyOutput

	return
}

// sendBlock responds to a block request with the desired block.
func (s *State) SendBlocks(conn net.Conn, data []byte) error {
	start := BlockHeight(DecUint64(data))
	end := s.Height()
	blocks := make([]Block, end-start)
	for i := range blocks {
		b := s.blockAtHeight(start + BlockHeight(i))
		if b == nil {
			return errors.New("unexpected nil block")
		}
		blocks[i] = *b
	}
	encBlocks := Marshal(blocks)
	encLen := EncUint64(uint64(len(encBlocks)))
	_, err := conn.Write(append(encLen[:4], encBlocks...))
	return err
}

// catchUp handles orphan blocks and situations where the node has fallen
// behind the longest fork.
func (s *State) catchUp(start BlockHeight) func(net.Conn) error {
	encbh := EncUint64(uint64(start))
	return func(conn net.Conn) error {
		conn.Write(append([]byte{'R', 4, 0, 0, 0}, encbh[:4]...))
		var blocks []Block
		encBlocks, err := ReadPrefix(conn)
		if err != nil {
			return err
		}
		if err = Unmarshal(encBlocks, &blocks); err != nil {
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

// Bootstrap requests blocks from peers until the full blockchain has been
// download.
func (s *State) Bootstrap() {
	addr := s.Server.RandomPeer()
	addr.Call(s.catchUp(0))
}
