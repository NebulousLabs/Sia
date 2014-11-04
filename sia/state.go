package sia

import (
	"sync"
)

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

	ConsensusState ConsensusState

	sync.Mutex
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
	Depth            BlockWeight   // Sum of weights of all blocks in this chain.

	ContractTerminations []*OpenContract
	MissedStorageProofs  []MissedStorageProof // Only need the output id because the only thing we do is delete the output.
}

// The ConsensusState is the state of the network on the current perceived
// longest fork. This gets updated as transactions are added, as blocks are
// added and reversed (in the event of a reorg).
type ConsensusState struct {
	CurrentBlock BlockID
	CurrentPath  map[BlockHeight]BlockID // Points to the block id for a given height.

	OpenContracts  map[ContractID]*OpenContract
	UnspentOutputs map[OutputID]Output
	SpentOutputs   map[OutputID]Output

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
}

// An open contract contains all information necessary to properly enforce a
// contract with no knowledge of the history of the contract.
type OpenContract struct {
	FileContract    FileContract
	ContractID      ContractID
	FundsRemaining  Currency
	Failures        uint32
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

// state.height() returns the height of the ConsensusState.
func (s *State) height() BlockHeight {
	return s.BlockMap[s.ConsensusState.CurrentBlock].Height
}

// state.blockAtHeight() returns the block from the current history at the
// input height.
func (s *State) blockAtHeight(height BlockHeight) (b *Block) {
	return s.BlockMap[s.ConsensusState.CurrentPath[height]].Block
}
