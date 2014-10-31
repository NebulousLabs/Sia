package sia

import (
	"time"
)

// Creates a new block.  This function creates a new block given a previous
// block, isn't happy with being interrupted.  Need a different thread that can
// be updated by listening on channels or something.
func (w *Wallet) GenerateBlock(state *State) (b *Block) {
	b = &Block{
		ParentBlock:  state.ConsensusState.CurrentBlock,
		Timestamp:    Timestamp(time.Now().Unix()),
		MinerAddress: w.CoinAddress,
	}

	// Add the transactions from the transaction pool.
	var transactionHashes []Hash
	for _, transaction := range state.ConsensusState.TransactionList {
		b.Transactions = append(b.Transactions, *transaction)
		transactionHashes = append(transactionHashes, HashBytes(Marshal(transaction)))
	}

	// Add the merkle root of the transactions.
	b.MerkleRoot = MerkleRoot(transactionHashes)

	// Perform work until the block matches the desired header value.
	err := state.validateHeader(state.BlockMap[state.ConsensusState.CurrentBlock], b)
	for err != nil {
		b.Nonce++
		err = state.validateHeader(state.BlockMap[state.ConsensusState.CurrentBlock], b)
	}

	return
}
