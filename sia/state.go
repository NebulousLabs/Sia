package sia

import (
	"math/big"
	"sync"
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
	RecentTimestamps [11]Timestamp // The 11 recent timestamps.
	Target           Target        // Target for next block.
	Depth            Target        // What the target would need to be to have a weight equal to all blocks up to this block.

	ContractTerminations []*OpenContract      // Contracts that terminated this block.
	MissedStorageProofs  []MissedStorageProof // Only need the output id because the only thing we do is delete the output.
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
	// FutureBlocks
	// OrphanBlocks

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
	CurrentBlock   BlockID
	CurrentPath    map[BlockHeight]BlockID // Points to the block id for a given height.
	OpenContracts  map[ContractID]*OpenContract
	UnspentOutputs map[OutputID]Output
	SpentOutputs   map[OutputID]Output // Useful for remembering how many coins an input had.

	// Mining Variables
	Mining     bool
	KillMining chan struct{}

	// A database of hosts that can be used to choose where to upload files.
	HostList    []Host
	TotalWeight Currency

	// Network Variables
	Server *TCPServer

	sync.Mutex // AcceptBlock() and AcceptTransaction() can be called concurrently.
}

// State.Height() returns the height of the longest fork.
func (s *State) Height() BlockHeight {
	return s.BlockMap[s.CurrentBlock].Height
}

// Depth() returns the depth of the current block of the state.
func (s *State) Depth() Target {
	return s.currentBlockNode().Depth
}

// State.currentBlockNode returns the node of the most recent block in the
// longest fork.
func (s *State) currentBlockNode() *BlockNode {
	return s.BlockMap[s.CurrentBlock]
}

// State.CurrentBlock returns the most recent block in the longest fork.
func (s *State) currentBlock() *Block {
	return s.BlockMap[s.CurrentBlock].Block
}

// State.blockAtHeight() returns the block from the current history at the
// input height.
func (s *State) blockAtHeight(height BlockHeight) (b *Block) {
	return s.BlockMap[s.CurrentPath[height]].Block
}

// State.currentBlockWeight() returns the weight of the current block in the
// heaviest fork.
func (s *State) currentBlockWeight() BlockWeight {
	return BlockWeight(new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).SetBytes(s.currentBlockNode().Target[:])))
}
