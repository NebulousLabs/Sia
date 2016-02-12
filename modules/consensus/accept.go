package consensus

import (
	"bytes"
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	errDoSBlock        = errors.New("block is known to be invalid")
	errNoBlockMap      = errors.New("block map is not in database")
	errInconsistentSet = errors.New("consensus set is not in a consistent state")
	errOrphan          = errors.New("block has no known parent")
)

// validateHeaderAndBlock does some early, low computation verification on the
// block. Callers should not assume that validation will happen in a particular
// order.
func (cs *ConsensusSet) validateHeaderAndBlock(tx dbTx, b types.Block) error {
	// See if the block is known already.
	id := b.ID()
	_, exists := cs.dosBlocks[id]
	if exists {
		return errDoSBlock
	}

	// Check if the block is already known.
	blockMap := tx.Bucket(BlockMap)
	if blockMap == nil {
		return errNoBlockMap
	}
	if blockMap.Get(id[:]) != nil {
		return modules.ErrBlockKnown
	}

	// Check for the parent.
	parentID := b.ParentID
	parentBytes := blockMap.Get(parentID[:])
	if parentBytes == nil {
		return errOrphan
	}
	var parent processedBlock
	err := cs.marshaler.Unmarshal(parentBytes, &parent)
	if err != nil {
		return err
	}
	// Check that the timestamp is not too far in the past to be acceptable.
	minTimestamp := cs.blockRuleHelper.minimumValidChildTimestamp(blockMap, &parent)

	return cs.blockValidator.ValidateBlock(b, minTimestamp, parent.ChildTarget, parent.Height+1)
}

// checkTarget returns true if the header's ID meets the given target.
func checkHeaderTarget(h types.BlockHeader, target types.Target) bool {
	blockHash := h.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// validateHeader does some early, low computation verification on the header
// to determine if the block should be downloaded. Callers should not assume
// that validation will happen in a particular order.
func (cs *ConsensusSet) validateHeader(tx dbTx, h types.BlockHeader) error {
	// See if the block is known already.
	id := h.ID()
	_, exists := cs.dosBlocks[id]
	if exists {
		return errDoSBlock
	}

	// Check if the block is already known.
	blockMap := tx.Bucket(BlockMap)
	if blockMap == nil {
		return errNoBlockMap
	}
	if blockMap.Get(id[:]) != nil {
		return modules.ErrBlockKnown
	}

	// Check for the parent.
	parentID := h.ParentID
	parentBytes := blockMap.Get(parentID[:])
	if parentBytes == nil {
		return errOrphan
	}
	var parent processedBlock
	err := cs.marshaler.Unmarshal(parentBytes, &parent)
	if err != nil {
		return err
	}

	// Check that the timestamp is not too far in the past to be acceptable.
	minTimestamp := cs.blockRuleHelper.minimumValidChildTimestamp(blockMap, &parent)
	if minTimestamp > h.Timestamp {
		return errEarlyTimestamp
	}

	// Check that the target of the new block is sufficient.
	if !checkHeaderTarget(h, parent.ChildTarget) {
		return modules.ErrBlockUnsolved
	}

	// TODO: check if the block is in the extreme or near future, and return
	// errExtremeFutureTimestamp or errFutureTimestamp, respectively.

	return nil
}

// addBlockToTree inserts a block into the blockNode tree by adding it to its
// parent's list of children. If the new blockNode is heavier than the current
// node, the blockchain is forked to put the new block and its parents at the
// tip. An error will be returned if block verification fails or if the block
// does not extend the longest fork.
//
// addBlockToTree must use its own database update because it might need to
// modify the database while returning an error on the block. To prevent error
// tracking complexity, the error is handled inside the function so that 'nil'
// can be appropriately returned by the database and the transaction can be
// committed. Switching to a managed tx through bolt will make this complexity
// unneeded.
func (cs *ConsensusSet) addBlockToTree(b types.Block) (ce changeEntry, err error) {
	var nonExtending bool
	err = cs.db.Update(func(tx *bolt.Tx) error {
		pb, err := getBlockMap(tx, b.ParentID)
		if build.DEBUG && err != nil {
			panic(err)
		}
		currentNode := currentProcessedBlock(tx)
		newNode := cs.newChild(tx, pb, b)

		// modules.ErrNonExtendingBlock should be returned if the block does
		// not extend the current blockchain, however the changes from newChild
		// should be comitted (which means 'nil' must be returned). A flag is
		// set to indicate that modules.ErrNonExtending should be returned.
		nonExtending = !newNode.heavierThan(currentNode)
		if nonExtending {
			return nil
		}
		var revertedBlocks, appliedBlocks []*processedBlock
		revertedBlocks, appliedBlocks, err = cs.forkBlockchain(tx, newNode)
		if err != nil {
			return err
		}
		for _, rn := range revertedBlocks {
			ce.RevertedBlocks = append(ce.RevertedBlocks, rn.Block.ID())
		}
		for _, an := range appliedBlocks {
			ce.AppliedBlocks = append(ce.AppliedBlocks, an.Block.ID())
		}
		// To have correct error handling, appendChangeLog must be called
		// before appending to the in-memory changelog. If this call fails, the
		// change is going to be reverted, but the in-memory changelog is not
		// going to be reverted.
		//
		// Technically, if bolt fails for some other reason (such as a
		// filesystem error), the in-memory changelog will be incorrect anyway.
		// Restarting Sia will fix it. The in-memory changelog is being phased
		// out.
		err = appendChangeLog(tx, ce)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return changeEntry{}, err
	}
	if nonExtending {
		return changeEntry{}, modules.ErrNonExtendingBlock
	}
	return ce, nil
}

// managedAcceptBlock will try to add a block to the consensus set. If the
// block does not extend the longest currently known chain, an error is
// returned but the block is still kept in memory. If the block extends a fork
// such that the fork becomes the longest currently known chain, the consensus
// set will reorganize itself to recognize the new longest fork. Accepted
// blocks are not relayed.
//
// Typically AcceptBlock should be used so that the accepted block is relayed.
// This method is typically only be used when there would otherwise be multiple
// consecutive calls to AcceptBlock with each successive call accepting the
// child block of the previous call.
func (cs *ConsensusSet) managedAcceptBlock(b types.Block) error {
	// Grab a lock on the consensus set. Lock is demoted later in the function,
	// failure to unlock before returning an error will cause a deadlock.
	cs.mu.Lock()

	// Start verification inside of a bolt View tx.
	err := cs.db.View(func(tx *bolt.Tx) error {
		// Do not accept a block if the database is inconsistent.
		if inconsistencyDetected(tx) {
			return errors.New("inconsistent database")
		}

		// Do some relatively inexpensive checks to validate the header and block.
		// Validation generally occurs in the order of least expensive validation
		// first.
		err := cs.validateHeaderAndBlock(boltTxWrapper{tx}, b)
		if err != nil {
			// If the block is in the near future, but too far to be acceptable, then
			// save the block and add it to the consensus set after it is no longer
			// too far in the future.
			if err == errFutureTimestamp {
				go func() {
					time.Sleep(time.Duration(b.Timestamp-(types.CurrentTimestamp()+types.FutureThreshold)) * time.Second)
					cs.AcceptBlock(b) // NOTE: Error is not handled.
				}()
			}
			return err
		}
		return nil
	})
	if err != nil {
		cs.mu.Unlock()
		return err
	}

	// Try adding the block to the block tree. This call will perform
	// verification on the block before adding the block to the block tree. An
	// error is returned if verification fails or if the block does not extend
	// the longest fork.
	changeEntry, err := cs.addBlockToTree(b)
	if err != nil {
		cs.mu.Unlock()
		return err
	}
	// If appliedBlocks is 0, revertedBlocks will also be 0.
	if build.DEBUG && len(changeEntry.AppliedBlocks) == 0 && len(changeEntry.RevertedBlocks) != 0 {
		panic("appliedBlocks and revertedBlocks are mismatched!")
	}

	// Updates complete, demote the lock.
	cs.mu.Demote()
	defer cs.mu.DemotedUnlock()
	if len(changeEntry.AppliedBlocks) > 0 {
		cs.readlockUpdateSubscribers(changeEntry)
	}
	return nil
}

// AcceptBlock will try to add a block to the consensus set. If the block does
// not extend the longest currently known chain, an error is returned but the
// block is still kept in memory. If the block extends a fork such that the
// fork becomes the longest currently known chain, the consensus set will
// reorganize itself to recognize the new longest fork. If a block is accepted
// without error, it will be relayed to all connected peers. This function
// should only be called for new blocks.
func (cs *ConsensusSet) AcceptBlock(b types.Block) error {
	err := cs.managedAcceptBlock(b)
	if err != nil {
		return err
	}
	// Broadcast the new block to all peers.
	peers := cs.gateway.Peers()
	go cs.gateway.Broadcast("RelayBlock", b, peers)
	return nil
}
