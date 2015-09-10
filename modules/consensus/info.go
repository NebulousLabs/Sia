package consensus

import (
	"math/big"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

// ConsensusSetInfo contains basic information about the ConsensusSet.
type ConsensusSetInfo struct {
	CurrentBlock types.BlockID
	Height       types.BlockHeight
	Target       types.Target
}

// currentBlockID returns the ID of the current block.
func (cs *ConsensusSet) currentBlockID() types.BlockID {
	return cs.db.getPath(cs.height())
}

func (cs *ConsensusSet) currentProcessedBlock() *processedBlock {
	return cs.db.getBlockMap(cs.currentBlockID())
}

// height returns the current height of the state.
func (cs *ConsensusSet) height() (bh types.BlockHeight) {
	_ = cs.db.Update(func(tx *bolt.Tx) error {
		bh = blockHeight(tx)
		return nil
	})
	return bh
}

// CurrentBlock returns the highest block on the tallest fork.
func (s *ConsensusSet) CurrentBlock() types.Block {
	counter := s.mu.RLock()
	defer s.mu.RUnlock(counter)
	return s.currentProcessedBlock().Block
}

// ChildTarget returns the target for the child of a block.
func (s *ConsensusSet) ChildTarget(bid types.BlockID) (target types.Target, exists bool) {
	// Lock is not needed because the values being read will not change once
	// they have been created.
	exists = s.db.inBlockMap(bid)
	if !exists {
		return
	}
	pb := s.db.getBlockMap(bid)
	target = pb.ChildTarget
	return
}

// EarliestChildTimestamp returns the earliest timestamp that the next block can
// have in order for it to be considered valid.
func (cs *ConsensusSet) EarliestChildTimestamp(bid types.BlockID) (timestamp types.Timestamp, exists bool) {
	// Lock is not needed because the values being read will not change once
	// they have been created.
	err := cs.db.View(func(tx *bolt.Tx) error {
		// Check that the parent exists.
		blockMap := tx.Bucket(BlockMap)

		// The identifier for the BlockMap is the sia encoding of the parent
		// id. The sia encoding is the same as ParentID[:].
		var parent processedBlock
		parentBytes := blockMap.Get(bid[:])
		if parentBytes == nil {
			return ErrOrphan
		}
		err := encoding.Unmarshal(parentBytes, &parent)
		if err != nil {
			return err
		}
		timestamp = earliestChildTimestamp(blockMap, &parent)
		return nil
	})
	if err != nil {
		return 0, false
	}
	return timestamp, true
}

// GenesisBlock returns the genesis block.
func (s *ConsensusSet) GenesisBlock() types.Block {
	lockID := s.mu.RLock()
	defer s.mu.RUnlock(lockID)
	return s.db.getBlockMap(s.db.getPath(0)).Block
}

// Height returns the height of the current blockchain (the longest fork).
func (s *ConsensusSet) Height() types.BlockHeight {
	counter := s.mu.RLock()
	defer s.mu.RUnlock(counter)
	return s.height()
}

// InCurrentPath returns true if the block presented is in the current path,
// false otherwise.
func (s *ConsensusSet) InCurrentPath(bid types.BlockID) bool {
	lockID := s.mu.RLock()
	defer s.mu.RUnlock(lockID)

	exists := s.db.inBlockMap(bid)
	if !exists {
		return false
	}
	node := s.db.getBlockMap(bid)
	return s.db.getPath(node.Height) == bid
}

// storageProofSegment returns the index of the segment that needs to be proven
// exists in a file contract.
func (cs *ConsensusSet) storageProofSegment(fcid types.FileContractID) (index uint64, err error) {
	err = cs.db.View(func(tx *bolt.Tx) error {
		// Check that the parent file contract exists.
		fcBucket := tx.Bucket(FileContracts)
		fcBytes := fcBucket.Get(fcid[:])
		if fcBytes == nil {
			return ErrUnrecognizedFileContractID
		}

		// Decode the file contract.
		var fc types.FileContract
		err := encoding.Unmarshal(fcBytes, &fc)
		if build.DEBUG && err != nil {
			panic(err)
		}

		// Get the trigger block id.
		blockPath := tx.Bucket(BlockPath)
		triggerHeight := fc.WindowStart - 1
		if triggerHeight > blockHeight(tx) {
			return ErrUnfinishedFileContract
		}
		var triggerID types.BlockID
		copy(triggerID[:], blockPath.Get(encoding.EncUint64(uint64(triggerHeight))))

		// Get the index by appending the file contract ID to the trigger block and
		// taking the hash, then converting the hash to a numerical value and
		// modding it against the number of segments in the file. The result is a
		// random number in range [0, numSegments]. The probability is very
		// slightly weighted towards the beginning of the file, but because the
		// size difference between the number of segments and the random number
		// being modded, the difference is too small to make any practical
		// difference.
		seed := crypto.HashAll(triggerID, fcid)
		numSegments := int64(crypto.CalculateLeaves(fc.FileSize))
		seedInt := new(big.Int).SetBytes(seed[:])
		index = seedInt.Mod(seedInt, big.NewInt(numSegments)).Uint64()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return index, nil
}

// StorageProofSegment returns the segment to be used in the storage proof for
// a given file contract.
func (cs *ConsensusSet) StorageProofSegment(fcid types.FileContractID) (index uint64, err error) {
	lockID := cs.mu.RLock()
	defer cs.mu.RUnlock(lockID)
	return cs.storageProofSegment(fcid)
}

// validStorageProofs checks that the storage proofs are valid in the context
// of the consensus set.
func (cs *ConsensusSet) validStorageProofs(t types.Transaction) error {
	for _, sp := range t.StorageProofs {
		// Check that the storage proof itself is valid.
		segmentIndex, err := cs.storageProofSegment(sp.ParentID)
		if err != nil {
			return err
		}

		fc := cs.db.getFileContracts(sp.ParentID) // previous function verifies the file contract exists
		leaves := crypto.CalculateLeaves(fc.FileSize)
		segmentLen := uint64(crypto.SegmentSize)
		if segmentIndex == leaves-1 {
			segmentLen = fc.FileSize % crypto.SegmentSize
		}

		// COMPATv0.4.0
		//
		// Fixing the padding situation resulted in a hardfork. The below code
		// will stop the hardfork from triggering before block 20,000.
		types.CurrentHeightLock.Lock()
		if (build.Release == "standard" && types.CurrentHeight < 21e3) || (build.Release == "testing" && types.CurrentHeight < 10) {
			segmentLen = uint64(crypto.SegmentSize)
		}
		types.CurrentHeightLock.Unlock()

		verified := crypto.VerifySegment(
			sp.Segment[:segmentLen],
			sp.HashSet,
			leaves,
			segmentIndex,
			fc.FileMerkleRoot,
		)
		if !verified {
			return ErrInvalidStorageProof
		}
	}

	return nil
}

// ValidStorageProofs checks that the storage proofs are valid in the context
// of the consensus set.
func (cs *ConsensusSet) ValidStorageProofs(t types.Transaction) (err error) {
	id := cs.mu.RLock()
	defer cs.mu.RUnlock(id)
	return cs.validStorageProofs(t)
}
