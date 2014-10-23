package sia

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

	BlockMap map[BlockID]Block // A list of all blocks in the blocktree.

	OpenTransactions map[TransactionID]Transaction // Transactions that are not yet incorporated into the ConsensusState.
	DeadTransactions map[TransactionID]Transaction // Transactions that spend outputs already in a block or open transaction.

	ConsensusState ConsensusState
}

type BlockNode struct {
	ID BlockID
	Children []BlockNode
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
