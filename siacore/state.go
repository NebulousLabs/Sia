package siacore

import (
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/NebulousLabs/Andromeda/encoding"
	"github.com/NebulousLabs/Andromeda/hash"
)

type (
	BlockWeight *big.Rat
)

// An open contract contains all information necessary to properly enforce a
// contract with no knowledge of the history of the contract.
type OpenContract struct {
	FileContract    FileContract
	ContractID      ContractID
	FundsRemaining  Currency
	Failures        uint64
	WindowSatisfied bool
}

// A missed storage proof indicates which contract missed the proof, and which
// output resulted from the missed proof. This is necessary because missed
// proofs are passive - they happen in the absense of a transaction, not in the
// presense of one. They must be stored in the block nodes so that a block can
// be correctly rewound without needing to scroll through the past
// 'ChallengeFrequency' blocks to figure out if a proof was missed or not.
type MissedStorageProof struct {
	OutputID   OutputID
	ContractID ContractID
}

// A BlockNode contains a block and the list of children to the block. Also
// contains some consensus information like which contracts have terminated and
// where there were missed storage proofs.
type BlockNode struct {
	Block    *Block
	Children []*BlockNode

	Height           BlockHeight
	Depth            Target        // What the target would need to be to have a weight equal to all blocks up to this block.
	Target           Target        // Target for next block.
	RecentTimestamps [11]Timestamp // The 11 recent timestamps.

	ContractTerminations []*OpenContract // Contracts that terminated this block.
	MissedStorageProofs  []MissedStorageProof
	SuccessfulWindows    []ContractID
}

// The state struct contains a list of all known blocks, sorted into a tree
// according to the shape of the network. It also contains the
// 'ConsensusState', which represents the state of consensus on the current
// longest fork.
type State struct {
	// The block root operates like a linked list of blocks, forming the
	// blocktree.
	blockRoot *BlockNode

	// One map for each potential type of block.
	badBlocks map[BlockID]struct{}           // A list of blocks that don't verify.
	blockMap  map[BlockID]*BlockNode         // A list of all blocks in the blocktree.
	orphanMap map[BlockID]map[BlockID]*Block // First map = ID of missing parent, second map = ID of orphan block.

	// The transaction pool works by storing a list of outputs that are
	// spent by transactions in the pool, and pointing to the transaction
	// that spends them. That makes it really easy to look up conflicts as
	// new transacitons arrive, and also easy to remove transactions from
	// the pool (delete every input used in the transaction.) The
	// transaction list contains only the first output, so that when
	// building blocks you can more easily iterate through every
	// transaction.
	transactionPool map[OutputID]*Transaction
	transactionList map[OutputID]*Transaction

	// Consensus Variables - the current state of consensus according to the
	// longest fork.
	currentBlockID BlockID
	currentPath    map[BlockHeight]BlockID // Points to the block id for a given height.
	unspentOutputs map[OutputID]Output
	openContracts  map[ContractID]*OpenContract
	spentOutputs   map[OutputID]Output // Useful for remembering how many coins an input had.

	// AcceptBlock() and AcceptTransaction() can be called concurrently.
	sync.Mutex
}

// State.Height() returns the height of the longest fork.
func (s *State) Height() BlockHeight {
	return s.blockMap[s.currentBlockID].Height
}

// State.Depth() returns the depth of the current block of the state.
func (s *State) Depth() Target {
	return s.currentBlockNode().Depth
}

// BlockAtHeight() returns the block from the current history at the
// input height.
func (s *State) BlockAtHeight(height BlockHeight) (b Block, err error) {
	if bn, ok := s.blockMap[s.currentPath[height]]; ok {
		b = *bn.Block
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

// CurrentBlock returns the most recent block in the longest fork.
func (s *State) CurrentBlock() Block {
	return *s.blockMap[s.currentBlockID].Block
}

// CurrentBlockWeight() returns the weight of the current block in the
// heaviest fork.
func (s *State) CurrentBlockWeight() BlockWeight {
	return BlockWeight(new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(s.currentBlockNode().Target[:])))
}

// CurrentEarliestLegalTimestamp returns the earliest legal timestamp of the
// next block - earlier timestamps will render the block invalid.
func (s *State) CurrentEarliestLegalTimestamp() Timestamp {
	return s.currentBlockNode().earliestLegalChildTimestamp()
}

// CurrentTarget returns the target of the next block that needs to be
// submitted to the state.
func (s *State) CurrentTarget() Target {
	return s.currentBlockNode().Target
}

func (s *State) SortedUtxoSet() (sortedOutputs []OutputID) {
	var unspentOutputStrings []string
	for outputID := range s.unspentOutputs {
		unspentOutputStrings = append(unspentOutputStrings, string(outputID[:]))
	}
	sort.Strings(unspentOutputStrings)

	for _, utxoString := range unspentOutputStrings {
		var outputID OutputID
		copy(outputID[:], utxoString)
		sortedOutputs = append(sortedOutputs, outputID)
	}
	return
}

// StateHash returns the markle root of the current state of consensus.
func (s *State) StateHash() hash.Hash {
	// Items of interest:
	// 1. CurrentBlockID
	// 2. Current Height
	// 3. Current Target
	// 4. Current Depth
	// 5. Earliest Allowed Timestamp of Next Block
	// 6. Genesis Block
	// 7. CurrentPath, ordered by height.
	// 8. UnspentOutputs, sorted by id.
	// 9. OpenContracts, sorted by id.

	// Create a slice of hashes representing all items of interest.
	var leaves []hash.Hash
	leaves = append(
		leaves,
		hash.Hash(s.currentBlockID),
		hash.HashBytes(encoding.Marshal(s.Height())),
		hash.HashBytes(encoding.Marshal(s.currentBlockNode().Target)),
		hash.HashBytes(encoding.Marshal(s.currentBlockNode().Depth)),
		hash.HashBytes(encoding.Marshal(s.currentBlockNode().earliestLegalChildTimestamp())),
		hash.Hash(s.blockRoot.Block.ID()),
	)

	// Add all the blocks in the current path.
	for i := 0; i < len(s.currentPath); i++ {
		leaves = append(leaves, hash.Hash(s.currentPath[BlockHeight(i)]))
	}

	// Sort the unspent outputs by the string value of their ID.
	sortedUtxos := s.SortedUtxoSet()

	// Add the unspent outputs in sorted order.
	for _, outputID := range sortedUtxos {
		leaves = append(leaves, hash.HashBytes(encoding.Marshal(s.unspentOutputs[outputID])))
	}

	// Sort the open contracts by the string value of their ID.
	var openContractStrings []string
	for contractID := range s.openContracts {
		openContractStrings = append(openContractStrings, string(contractID[:]))
	}
	sort.Strings(openContractStrings)

	// Add the open contracts in sorted order.
	for _, stringContractID := range openContractStrings {
		var contractID ContractID
		copy(contractID[:], []byte(stringContractID))
		leaves = append(leaves, hash.HashBytes(encoding.Marshal(s.openContracts[contractID])))
	}

	return hash.MerkleRoot(leaves)
}

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
