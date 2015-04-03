package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrBadBlock        = errors.New("block is known to be invalid")
	ErrBlockKnown      = errors.New("block exists in block map")
	ErrEarlyTimestamp  = errors.New("block timestamp is too early")
	ErrFutureTimestamp = errors.New("block timestamp too far in future")
	ErrOrphan          = errors.New("block has no known parent")
	ErrLargeBlock      = errors.New("block is too large to be accepted")
	ErrMinerPayout     = errors.New("miner payout sum does not equal block subsidy")
	ErrMissedTarget    = errors.New("block does not meet target")
)

// checkMinerPayouts verifies that the sum of all the miner payouts is equal to
// the block subsidy (which is the coinbase + miner fees).
func (s *State) checkMinerPayouts(b types.Block) (err error) {
	// Sanity check - the block's parent needs to exist and be known.
	parentNode, exists := s.blockMap[b.ParentID]
	if !exists {
		if build.DEBUG {
			panic("misuse of checkMinerPayouts - block has no known parent")
		}
		return ErrOrphan
	}

	// Find the total subsidy for the miners: coinbase + fees.
	subsidy := types.CalculateCoinbase(parentNode.height + 1)
	for _, txn := range b.Transactions {
		for _, fee := range txn.MinerFees {
			subsidy = subsidy.Add(fee)
		}
	}

	// Find the sum of the miner payouts.
	var payoutSum types.Currency
	for _, payout := range b.MinerPayouts {
		if payout.Value.IsZero() {
			return errors.New("cannot have zero or negative miner payout")
		}
		payoutSum = payoutSum.Add(payout.Value)
	}

	// Return an error if the subsidy isn't equal to the payouts.
	if subsidy.Cmp(payoutSum) != 0 {
		return ErrMinerPayout
	}

	return
}

// validHeader does some early, low computation verification on the block.
func (s *State) validHeader(b types.Block) (err error) {
	// Grab the parent of the block.
	parent, exists := s.blockMap[b.ParentID]
	if !exists {
		return ErrOrphan
	}

	// Check the ID meets the target. This is one of the earliest checks to
	// enforce that blocks need to have committed to a large amount of work
	// before being verified - a DoS protection.
	if !b.CheckTarget(parent.target) {
		return ErrMissedTarget
	}

	// Check that the block is the correct size.
	if uint64(len(encoding.Marshal(b))) > types.BlockSizeLimit {
		return ErrLargeBlock
	}

	// If timestamp is too far in the past, reject and put in bad blocks.
	if parent.earliestChildTimestamp() > b.Timestamp {
		return ErrEarlyTimestamp
	}

	// Check that the block is not too far in the future. An external process
	// will need to be responsible for resubmitting the block once it is no
	// longer in the future.
	if b.Timestamp > types.CurrentTimestamp()+types.FutureThreshold {
		return ErrFutureTimestamp
	}

	// Verify that the miner payouts sum to the total amount of fees allowed to
	// be collected by the miners.
	err = s.checkMinerPayouts(b)
	if err != nil {
		return
	}

	return
}

// addBlockToTree inserts a block into the blockNode tree by adding it to its
// parent's list of children. If the new blockNode is heavier than the current
// node, the blockchain is forked.
func (s *State) addBlockToTree(b types.Block) (err error) {
	parentNode := s.blockMap[b.ParentID]
	newNode := parentNode.newChild(b)

	// Add the node to the block map
	s.blockMap[b.ID()] = newNode

	if newNode.heavierThan(s.currentBlockNode()) {
		err = s.forkBlockchain(newNode)
		if err != nil {
			return
		}
	}

	return
}

// AcceptBlock will add blocks to the state, forking the blockchain if they are
// on a fork that is heavier than the current fork.
func (s *State) AcceptBlock(b types.Block) (err error) {
	counter := s.mu.Lock()
	defer s.mu.Unlock(counter)

	// Check maps for information about the block.
	_, exists := s.badBlocks[b.ID()]
	if exists {
		return ErrBadBlock
	}
	_, exists = s.blockMap[b.ID()]
	if exists {
		return ErrBlockKnown
	}

	err = s.validHeader(b)
	if err != nil {
		return
	}

	err = s.addBlockToTree(b)
	if err != nil {
		return
	}

	return
}
