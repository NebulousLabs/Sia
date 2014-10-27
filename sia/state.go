package sia

// A transaction will not make it into the txn pool unless all of the signatures have been verified.
// That that's left then is to verify that the outputs are unused.

// state.go manages the state of the network, which in simplified terms is all
// the blocks ever seen combined with some understanding of how they fit
// together.
//
// For the time being I've made it so that the state struct just stores
// everything, instead of using pointers.
type State struct {
	// The block root operates like a linked list of blocks, forming the
	// blocktree.  Blocks can never be removed from the tree, so this doesn't
	// need to be pointers.
	BlockRoot BlockNode

	BadBlocks map[BlockID]struct{} // A list of blocks that don't verify.
	BlockMap map[BlockID]*BlockNode // A list of all blocks in the blocktree.
	// FutureBlocks map[BlockID]Block // A list of blocks with out-of-range timestamps.
	// OrphanBlocks map[BlockID]Block // A list of all blocks that are orphans.

	OpenTransactions map[TransactionID]Transaction // Transactions that are not yet incorporated into the ConsensusState.
	// DeadTransactions map[TransactionID]Transaction // Transactions that spend outputs already in a block or open transaction.

	ConsensusState ConsensusState
}

type BlockNode struct {
	Block Block
	// Verified bool // indicates whether the computation has been done to ensure all txns make sense.
	Height uint32 // could be computed, but this is easier.
	Children []*BlockNode

	// A list of contract outputs that have been spent at this point, plus
	// miner payment output.
}

type ConsensusState struct {
	UnspentOutputs map[OutputID]Output
	OpenContracts map[ContractID]OpenContract
}

type OpenContract struct {
	Contract FileContract
	RemainingFunds Currency
	CurrentWindow bool // true means that a proof has been seen for the current window, false means it hasn't.
}
