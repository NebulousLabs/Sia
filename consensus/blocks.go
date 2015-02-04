package consensus

import (
	"errors"
	"math/big"
	"sort"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
)

// A non-consensus rule that dictates how much heavier a competing chain has to
// be before the node will switch to mining on that chain. The percent refers
// to the percent of the weight of the most recent block on the winning chain,
// not the weight of the entire chain.
//
// This rule is in place because the difficulty gets updated every block, and
// that means that of two competing blocks, one could be very slightly heavier.
// The slightly heavier one should not be switched to if it was not seen first,
// because the amount of extra weight in the chain is inconsequential. The
// maximum difficulty shift will prevent people from manipulating timestamps
// enough to produce a block that is substantially heavier.
var (
	SurpassThreshold = big.NewRat(50, 100)
)

// Exported Errors
var (
	BlockKnownErr     = errors.New("block exists in block map.")
	EarlyTimestampErr = errors.New("block timestamp is too early, block is illegal.")
	FutureBlockErr    = errors.New("timestamp too far in future, will try again later.")
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
	if DEBUG {
		if !exists {
			panic("parent node doesn't exist in block map when calling checkMinerPayouts")
		}
	}

	// Find the allowed miner subsidy.
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
	if subsidy != payoutSum {
		err = MinerPayoutErr
		return
	}

	return
}

// validHeader returns err = nil if the header information in the block is
// valid, and returns an error otherwise.
func (s *State) validHeader(b Block) (err error) {
	parent := s.blockMap[b.ParentID]
	// Check the id meets the target.
	if !b.CheckTarget(parent.target) {
		return MissedTargetErr
	}

	// If timestamp is too far in the past, reject and put in bad blocks.
	if parent.earliestChildTimestamp() > b.Timestamp {
		return EarlyTimestampErr
	}

	// Check that the block is not too far in the future.
	skew := b.Timestamp - Timestamp(time.Now().Unix())
	if skew > FutureThreshold {
		return FutureBlockErr
	}

	// Check the miner payouts.
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

	// See if the block is a known invalid block.
	_, exists := s.badBlocks[b.ID()]
	if exists {
		err = errors.New("block is known to be invalid")
		return
	}

	// See if the block is already known and valid.
	_, exists = s.blockMap[b.ID()]
	if exists {
		err = BlockKnownErr
		return
	}

	// See if the block is an orphan.
	_, exists = s.blockMap[b.ParentID]
	if !exists {
		err = OrphanErr
		return
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
