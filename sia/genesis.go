package sia

import (
	"net"
	"time"
)

const (
	GenesisSubsidy = Currency(25000)
)

// These values will be generated before release, but the code for generating
// them will never be released.  All that the rest of the world will see is
// hardcoded values.
func createGenesisBlock(premineAddress CoinAddress) (b *Block) {
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
	genesisBlock := createGenesisBlock(premineAddress)

	// Fill out the block root node, and add it to the BlockMap.
	s.BlockRoot.Block = genesisBlock
	s.BlockRoot.Height = 0
	for i := range s.BlockRoot.RecentTimestamps {
		s.BlockRoot.RecentTimestamps[i] = Timestamp(time.Now().Unix())
	}
	s.BlockRoot.Target[1] = 16
	s.BlockRoot.Depth[0] = 255 // depth of genesis block is set to 111111110000000000000000...
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

// requestBlock returns a closure that can be used with addr.Call to request a
// block at a specific height.
func (s *State) requestBlock(bh BlockHeight) func(net.Conn) error {
	encbh := EncUint64(uint64(bh))
	return func(conn net.Conn) error {
		conn.Write(append([]byte{'R', 4, 0, 0, 0}, encbh[:4]...))
		conn.Read()
		b, err := Unmarshal(blockData)
		if err != nil {
			return err
		}
		return s.AcceptBlock(b)
	}
}

// sendBlock responds to a block request with the desired block
func (s *State) sendBlock(conn net.Conn, data []byte) error {
	height := BlockHeight(DecUint64(data))
	b := s.blockAtHeight(height)
	if b == nil {
		return errors.New("invalid block height")
	}
	encBlock := Marshal(val)
	encLen := EncUint64(uint64(len(encBlock)))
	_, err = conn.Write(append(encLen[:4], encBlock...))
	return err
}

// Bootstrap requests blocks from peers until the full blockchain has been download.
func (s *State) Bootstrap() {
	i := BlockHeight(0)
	for { // when do we break?
		s.tcps.Broadcast(s.requestBlock(i))
		i++
	}
}
