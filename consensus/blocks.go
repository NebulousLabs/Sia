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
	BlockKnownErr    = errors.New("block exists in block map.")
	FutureBlockErr   = errors.New("timestamp too far in future, will try again later.")
	KnownOrphanErr   = errors.New("block is a known orphan")
	UnknownOrphanErr = errors.New("block is an unknown orphan")
)

// handleOrphanBlock adds a block to the list of orphans, returning an error
// indicating whether the orphan existed previously or not. handleOrphanBlock
// always returns an error.
func (s *State) handleOrphanBlock(b Block) error {
	// Sanity check - block must be an orphan!
	if DEBUG {
		_, exists := s.blockMap[b.ParentBlockID]
		if exists {
			panic("Incorrect use of handleOrphanBlock")
		}
	}

	// Check if the missing parent is unknown
	missingParent, exists := s.missingParents[b.ParentBlockID]
	if !exists {
		// Add an entry for the parent and add the orphan block to the entry.
		s.missingParents[b.ParentBlockID] = make(map[BlockID]Block)
		s.missingParents[b.ParentBlockID][b.ID()] = b
		return UnknownOrphanErr
	}

	// Check if the orphan is already known, and add the orphan if not.
	_, exists = missingParent[b.ID()]
	if exists {
		return KnownOrphanErr
	}
	missingParent[b.ID()] = b
	return UnknownOrphanErr
}

// earliestChildTimestamp returns the earliest timestamp that a child node
// can have while still being valid. See section 'Timestamp Rules' in
// Consensus.md.
func (bn *BlockNode) earliestChildTimestamp() Timestamp {
	// Get the previous `MedianTimestampWindow` timestamps.
	var intTimestamps []int
	referenceNode := bn
	for i := 0; i < MedianTimestampWindow; i++ {
		intTimestamps = append(intTimestamps, int(referenceNode.Block.Timestamp))
		if referenceNode.Parent != nil {
			referenceNode = referenceNode.Parent
		}
	}
	sort.Ints(intTimestamps)

	// Return the median of the sorted timestamps.
	return Timestamp(intTimestamps[MedianTimestampWindow/2])
}

// validHeader returns err = nil if the header information in the block is
// valid, and returns an error otherwise.
func (s *State) validHeader(b Block) (err error) {
	parent := s.blockMap[b.ParentBlockID]
	// Check the id meets the target.
	if !b.CheckTarget(parent.Target) {
		err = errors.New("block does not meet target")
		return
	}

	// If timestamp is too far in the past, reject and put in bad blocks.
	if parent.earliestChildTimestamp() > b.Timestamp {
		err = errors.New("timestamp invalid for being in the past")
		return
	}

	// Check that the block is not too far in the future.
	skew := b.Timestamp - Timestamp(time.Now().Unix())
	if skew > FutureThreshold {
		err = FutureBlockErr
		return
	}

	// Check that the block is the correct size.
	encodedBlock := encoding.Marshal(b)
	if len(encodedBlock) > BlockSizeLimit {
		err = errors.New("Block is too large, will not be accepted.")
		return
	}

	// Check that the transaction merkle root matches the transactions
	// included into the block.
	if b.MerkleRoot != b.TransactionMerkleRoot() {
		err = errors.New("merkle root does not match transactions sent.")
		return
	}

	return
}

// State.AcceptBlock() will add blocks to the state, forking the blockchain if
// they are on a fork that is heavier than the current fork.
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
	_, exists = s.blockMap[b.ParentBlockID]
	if !exists {
		err = s.handleOrphanBlock(b)
		return
	}

	err = s.addBlockToTree(b)
	if err != nil {
		return
	}

	// Notify subscribers that the blockchain has updated.
	go s.threadedNotifySubscribers()

	return
}
