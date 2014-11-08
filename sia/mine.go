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

// Creates a block that is ready for nonce grinding.
func (s *State) blockForWork(minerAddress CoinAddress) (b *Block, target Target) {
	b = &Block{
		ParentBlock:  s.ConsensusState.CurrentBlock,
		Timestamp:    Timestamp(time.Now().Unix()),
		MinerAddress: minerAddress,
	}
	// IF TIMESTAMP IS INVALID, CREATE A VALID TIMESTAMP! --> 'future median' attack.

	// Add the transactions from the transaction pool.
	for _, transaction := range s.ConsensusState.TransactionList {
		b.Transactions = append(b.Transactions, *transaction)
	}
	b.MerkleRoot = b.expectedTransactionMerkleRoot()

	// Determine the target for the block.
	target = s.currentBlockNode().Target

	return
}

// Tries to find a solution by increasing the nonce and checking the hash
// repeatedly.
func solveBlock(b *Block, target Target) bool {
	for i := 0; i < IterationsPerAttempt; i++ {
		if b.checkTarget(target) {
			return true
		}

		b.Nonce++
	}

	return false
}

// Creates a new block.  This function creates a new block given a previous
// block, isn't happy with being interrupted.  Need a different thread that can
// be updated by listening on channels or something.
func (s *State) GenerateBlock(minerAddress CoinAddress) (b *Block) {
	for {
		var err error
		b, err = s.AttemptToGenerateBlock(minerAddress)
		if err == nil {
			return b
		}
	}
}

// AttemptToGenerateBlock attempts to generate a block, but instead of running
// until a block is found, it just tries a single time.
func (s *State) AttemptToGenerateBlock(minerAddress CoinAddress) (b *Block, err error) {
	s.Lock()
	b, target := s.blockForWork(minerAddress)
	s.Unlock()

	if solveBlock(b, target) {
		return b, nil
	} else {
		err = errors.New("could not find block")
		return
	}
}

// ToggleMining creates a channel and mines until it receives a kill signal.
func (s *State) ToggleMining(minerAddress CoinAddress) (b *Block) {
	if !s.Mining {
		s.KillMining = make(chan struct{})
		s.Mining = true
	}

	// Need some channel to wait on to kill the function.
	for {
		select {
		case <-s.KillMining:
			return

		default:
			block, err := s.AttemptToGenerateBlock(minerAddress)
			if err == nil {
				s.AcceptBlock(*block)
			}
		}
	}
}
