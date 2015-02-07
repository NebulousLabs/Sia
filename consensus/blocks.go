package consensus

import (
	"errors"
	"sort"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
)

// Exported Errors
var (
	BadBlockErr       = errors.New("block is known to be invalid.")
	BlockKnownErr     = errors.New("block exists in block map.")
	EarlyTimestampErr = errors.New("block timestamp is too early, block is illegal.")
	FutureBlockErr    = errors.New("block timestamp too far in future")
	OrphanErr         = errors.New("block has no known parent")
	LargeBlockErr     = errors.New("block is too large to be accepted")
	MinerPayoutErr    = errors.New("miner payout sum does not equal block subsidy")
	MissedTargetErr   = errors.New("block does not meet target")
)

// earliestChildTimestamp returns the earliest timestamp that a child node
// can have while still being valid. See section 'Timestamp Rules' in
// Consensus.md.
func (bn *blockNode) earliestChildTimestamp() Timestamp {
	// Get the previous `MedianTimestampWindow` timestamps.
	var intTimestamps []int
	referenceNode := bn
	for i := 0; i < MedianTimestampWindow; i++ {
		intTimestamps = append(intTimestamps, int(referenceNode.block.Timestamp))
		if referenceNode.parent != nil {
			referenceNode = referenceNode.parent
		}
	}
	sort.Ints(intTimestamps)

	// Return the median of the sorted timestamps.
	return Timestamp(intTimestamps[MedianTimestampWindow/2])
}

// checkMinerPayouts verifies that the sum of all the miner payouts is equal to
// the block subsidy (which is the coinbase + miner fees).
func (s *State) checkMinerPayouts(b Block) (err error) {
	// Sanity check - the block's parent needs to exist and be known.
	parentNode, exists := s.blockMap[b.ParentID]
	if !exists {
		if DEBUG {
			panic("misuse of checkMinerPayouts - block has no known parent")
		} else {
			return OrphanErr
		}
	}

	// Find the total subsidy for the miners: coinbase + fees.
	subsidy := CalculateCoinbase(parentNode.height + 1)
	for _, txn := range b.Transactions {
		for _, fee := range txn.MinerFees {
			err = subsidy.Add(fee)
			if err != nil {
				return
			}
		}
	}

	// Find the sum of the miner payouts.
	var payoutSum Currency
	for _, payout := range b.MinerPayouts {
		err = payoutSum.Add(payout.Value)
		if err != nil {
			return
		}
	}

	// Return an error if the subsidy isn't equal to the payouts.
	if subsidy.Cmp(payoutSum) != 0 {
		return MinerPayoutErr
	}

	return
}

// validHeader does some early, low computation verification on the block.
func (s *State) validHeader(b Block) (err error) {
	// Grab the parent of the block.
	parent, exists := s.blockMap[b.ParentID]
	if !exists {
		return OrphanErr
	}

	// Check the id meets the target. This is one of the earliest checks to
	// enforce that blocks need to have committed to a large amount of work
	// before being verified - a DoS protection.
	if !b.CheckTarget(parent.target) {
		return MissedTargetErr
	}

	// If timestamp is too far in the past, reject and put in bad blocks.
	if parent.earliestChildTimestamp() > b.Timestamp {
		return EarlyTimestampErr
	}

	// Check that the block is not too far in the future. An external process
	// will need to be responsible for resubmitting the block once it is no
	// longer in the future.
	skew := int(b.Timestamp) - int(Timestamp(time.Now().Unix()))
	if skew > FutureThreshold {
		return FutureBlockErr
	}

	// Verify that the miner payouts sum to the total amount of fees allowed to
	// be collected by the miners.
	err = s.checkMinerPayouts(b)
	if err != nil {
		return
	}

	// Check that the block is the correct size.
	encodedBlock := encoding.Marshal(b)
	if len(encodedBlock) > BlockSizeLimit {
		return LargeBlockErr
	}

	return
}

// AcceptBlock will add blocks to the state, forking the blockchain if they are
// on a fork that is heavier than the current fork.
func (s *State) AcceptBlock(b Block) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check maps for information about the block.
	_, exists := s.badBlocks[b.ID()]
	if exists {
		return BadBlockErr
	}
	_, exists = s.blockMap[b.ID()]
	if exists {
		return BlockKnownErr
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
