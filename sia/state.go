package sia

// A transaction will not make it into the txn pool unless all of the
// signatures have been verified.  That that's left then is to verify that the
// outputs are unused.

// state.go manages the state of the network, which in simplified terms is all
// the blocks ever seen combined with some understanding of how they fit
// together.
//
// For the time being I've made it so that the state struct just stores
// everything, instead of using pointers.
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
}

type BlockNode struct {
	Block    *Block
	Children []*BlockNode

	Height           BlockHeight
	RecentTimestamps [11]Timestamp // The 11 recent timestamps.
	Target           Target        // Target for next block.
	Depth            BlockWeight   // Sum of weights of all blocks in this chain.
}

type ConsensusState struct {
	CurrentBlock BlockID
	CurrentPath  map[BlockHeight]BlockID // Points to the block id for a given height.

	UnspentOutputs map[OutputID]Output
	SpentOutputs   map[OutputID]Output

	// The transaction pool works by storing a list of outputs that are
	// spent by transactions in the pool, and pointing to the transaction
	// that spends them. That makes it really easy to look up conflicts as
	// new transacitons arrive, and also easy to remove transactions from
	// the pool (delete every input used in the transaction.)
	TransactionPool map[OutputID]*Transaction
}
