package siacore

import (
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
	BlockRoot *BlockNode

	// One map for each potential type of block.
	BadBlocks map[BlockID]struct{}   // A list of blocks that don't verify.
	BlockMap  map[BlockID]*BlockNode // A list of all blocks in the blocktree.

	// The transaction pool works by storing a list of outputs that are
	// spent by transactions in the pool, and pointing to the transaction
	// that spends them. That makes it really easy to look up conflicts as
	// new transacitons arrive, and also easy to remove transactions from
	// the pool (delete every input used in the transaction.) The
	// transaction list contains only the first output, so that when
	// building blocks you can more easily iterate through every
	// transaction.
	TransactionPool map[OutputID]*Transaction
	TransactionList map[OutputID]*Transaction

	// Consensus Variables - the current state of consensus according to the
	// longest fork.
	CurrentBlockID BlockID
	CurrentPath    map[BlockHeight]BlockID // Points to the block id for a given height.
	UnspentOutputs map[OutputID]Output
	OpenContracts  map[ContractID]*OpenContract
	SpentOutputs   map[OutputID]Output // Useful for remembering how many coins an input had.

	// AcceptBlock() and AcceptTransaction() can be called concurrently.
	sync.Mutex
}

// State.Height() returns the height of the longest fork.
func (s *State) Height() BlockHeight {
	return s.BlockMap[s.CurrentBlockID].Height
}

// Depth() returns the depth of the current block of the state.
func (s *State) Depth() Target {
	return s.CurrentBlockNode().Depth
}

// State.currentBlockNode returns the node of the most recent block in the
// longest fork.
func (s *State) CurrentBlockNode() *BlockNode {
	return s.BlockMap[s.CurrentBlockID]
}

// State.CurrentBlock returns the most recent block in the longest fork.
func (s *State) CurrentBlock() *Block {
	return s.BlockMap[s.CurrentBlockID].Block
}

// State.blockAtHeight() returns the block from the current history at the
// input height.
func (s *State) BlockAtHeight(height BlockHeight) (b *Block) {
	return s.BlockMap[s.CurrentPath[height]].Block
}

// State.currentBlockWeight() returns the weight of the current block in the
// heaviest fork.
func (s *State) CurrentBlockWeight() BlockWeight {
	return BlockWeight(new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(s.CurrentBlockNode().Target[:])))
}

// StateHash returns the markle root of the current state of consensus.
func (s *State) StateHash() hash.Hash {
	// Items of interest:
	// 1. Current Height
	// 2. Current Target
	// 3. Current Depth
	// 4. Earliest Allowed Timestamp of Next Block
	// 5. Genesis Block
	// 6. CurrentBlockID
	// 7. CurrentPath, ordered by height.
	// 8. UnspentOutputs, sorted by id.
	// 9. OpenContracts, sorted by id.

	// Create a slice of hashes representing all items of interest.
	var leaves []hash.Hash
	leaves = append(
		leaves,
		hash.HashBytes(encoding.Marshal(s.Height())),
		hash.HashBytes(encoding.Marshal(s.CurrentBlockNode().Target)),
		hash.HashBytes(encoding.Marshal(s.CurrentBlockNode().Depth)),
		hash.HashBytes(encoding.Marshal(s.CurrentBlockNode().EarliestLegalChildTimestamp())),
		hash.Hash(s.BlockRoot.Block.ID()),
		hash.Hash(s.CurrentBlockID),
	)

	// Add all the blocks in the current path.
	for i := 0; i < len(s.CurrentPath); i++ {
		leaves = append(leaves, hash.Hash(s.CurrentPath[BlockHeight(i)]))
	}

	// Sort the unspent outputs by the string value of their ID.
	var unspentOutputStrings []string
	for outputID := range s.UnspentOutputs {
		unspentOutputStrings = append(unspentOutputStrings, string(outputID[:]))
	}
	sort.Strings(unspentOutputStrings)

	// Add the unspent outputs in sorted order.
	for _, stringOutputID := range unspentOutputStrings {
		var outputID OutputID
		copy(outputID[:], []byte(stringOutputID))
		leaves = append(leaves, hash.HashBytes(encoding.Marshal(s.UnspentOutputs[outputID])))
	}

	// Sort the open contracts by the string value of their ID.
	var openContractStrings []string
	for contractID := range s.OpenContracts {
		openContractStrings = append(openContractStrings, string(contractID[:]))
	}
	sort.Strings(unspentOutputStrings)

	// Add the open contracts in sorted order.
	for _, stringContractID := range openContractStrings {
		var contractID ContractID
		copy(contractID[:], []byte(stringContractID))
		leaves = append(leaves, hash.HashBytes(encoding.Marshal(s.OpenContracts[contractID])))
	}

	return hash.MerkleRoot(leaves)
}
