package sia

import (
	"errors"
	"time"
)

const (
	// If it takes less than 1 second to go through all of the iterations,
	// then repeat work will be performed.
	IterationsPerAttempt = 10 * 1000 * 1000
)

func (s *State) blockForWork(minerAddress CoinAddress) (b *Block, target Target) {
	b = &Block{
		ParentBlock:  s.ConsensusState.CurrentBlock,
		Timestamp:    Timestamp(time.Now().Unix()),
		MinerAddress: minerAddress,
	}

	// Add the transactions from the transaction pool.
	for _, transaction := range s.ConsensusState.TransactionList {
		b.Transactions = append(b.Transactions, *transaction)
	}
	b.MerkleRoot = b.expectedMerkleRoot()

	// Determine the target for the block.
	target = s.BlockMap[s.ConsensusState.CurrentBlock].Target

	return
}

func solveBlock(b *Block, target Target) (err error) {
	for i := 0; i < IterationsPerAttempt; i++ {
		if b.checkTarget(target) {
			return
		}

		b.Nonce++
	}

	err = errors.New("did not find winning block")
	return
}

// Creates a new block.  This function creates a new block given a previous
// block, isn't happy with being interrupted.  Need a different thread that can
// be updated by listening on channels or something.
func (s *State) GenerateBlock(minerAddress CoinAddress) (b *Block) {
	var target Target
	err := errors.New("getting started")
	for err != nil {
		s.Lock()
		b, target = s.blockForWork(minerAddress)
		s.Unlock()

		err = solveBlock(b, target)
	}

	return
}
