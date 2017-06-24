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
	errNonLinearChain  = errors.New("block set is not a contiguous chain")
)

// managedBroadcastBlock will broadcast a block to the consensus set's peers.
func (cs *ConsensusSet) managedBroadcastBlock(b types.Block) {
	// broadcast the block header to all peers
	go cs.gateway.Broadcast("RelayHeader", b.Header(), cs.gateway.Peers())
}

// validateHeaderAndBlock does some early, low computation verification on the
// block. Callers should not assume that validation will happen in a particular
// order.
func (cs *ConsensusSet) validateHeaderAndBlock(tx dbTx, b types.Block) (parent *processedBlock, err error) {
	// Check if the block is a DoS block - a known invalid block that is expensive
	// to validate.
	id := b.ID()
	_, exists := cs.dosBlocks[id]
	if exists {
		return nil, errDoSBlock
	}

	// Check if the block is already known.
	blockMap := tx.Bucket(BlockMap)
	if blockMap == nil {
		return nil, errNoBlockMap
	}
	if blockMap.Get(id[:]) != nil {
		return nil, modules.ErrBlockKnown
	}

	// Check for the parent.
	parentID := b.ParentID
	parentBytes := blockMap.Get(parentID[:])
	if parentBytes == nil {
		return nil, errOrphan
	}
	parent = new(processedBlock)
	err = cs.marshaler.Unmarshal(parentBytes, parent)
	if err != nil {
		return nil, err
	}
	// Check that the timestamp is not too far in the past to be acceptable.
	minTimestamp := cs.blockRuleHelper.minimumValidChildTimestamp(blockMap, parent)

	err = cs.blockValidator.ValidateBlock(b, minTimestamp, parent.ChildTarget, parent.Height+1, cs.log)
	if err != nil {
		return nil, err
	}
	return parent, nil
}

// checkHeaderTarget returns true if the header's ID meets the given target.
func checkHeaderTarget(h types.BlockHeader, target types.Target) bool {
	blockHash := h.ID()
	return bytes.Compare(target[:], blockHash[:]) >= 0
}

// validateHeader does some early, low computation verification on the header
// to determine if the block should be downloaded. Callers should not assume
// that validation will happen in a particular order.
func (cs *ConsensusSet) validateHeader(tx dbTx, h types.BlockHeader) error {
	// Check if the block is a DoS block - a known invalid block that is expensive
	// to validate.
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

	// Check that the target of the new block is sufficient.
	if !checkHeaderTarget(h, parent.ChildTarget) {
		return modules.ErrBlockUnsolved
	}

	// TODO: check if the block is a non extending block once headers-first
	// downloads are implemented.

	// Check that the timestamp is not too far in the past to be acceptable.
	minTimestamp := cs.blockRuleHelper.minimumValidChildTimestamp(blockMap, &parent)
	if minTimestamp > h.Timestamp {
		return errEarlyTimestamp
	}

	// Check if the block is in the extreme future. We make a distinction between
	// future and extreme future because there is an assumption that by the time
	// the extreme future arrives, this block will no longer be a part of the
	// longest fork because it will have been ignored by all of the miners.
	if h.Timestamp > types.CurrentTimestamp()+types.ExtremeFutureThreshold {
		return errExtremeFutureTimestamp
	}

	// We do not check if the header is in the near future here, because we want
	// to get the corresponding block as soon as possible, even if the block is in
	// the near future.

	return nil
}

// addBlockToTree inserts a block into the blockNode tree by adding it to its
// parent's list of children. If the new blockNode is heavier than the current
// node, the blockchain is forked to put the new block and its parents at the
// tip. An error will be returned if block verification fails or if the block
// does not extend the longest fork.
//
// addBlockToTree might need to modify the database while returning an error
// on the block. Such errors are handled outside of the transaction by the
// caller. Switching to a managed tx through bolt will make this complexity
// unneeded.
func (cs *ConsensusSet) addBlockToTree(tx *bolt.Tx, b types.Block, parent *processedBlock) (ce changeEntry, err error) {
	// Prepare the child processed block associated with the parent block.
	newNode := cs.newChild(tx, parent, b)

	// Check whether the new node is part of a chain that is heavier than the
	// current node. If not, return ErrNonExtending and don't fork the
	// blockchain.
	currentNode := currentProcessedBlock(tx)
	if !newNode.heavierThan(currentNode) {
		return changeEntry{}, modules.ErrNonExtendingBlock
	}

	// Fork the blockchain and put the new heaviest block at the tip of the
	// chain.
	var revertedBlocks, appliedBlocks []*processedBlock
	revertedBlocks, appliedBlocks, err = cs.forkBlockchain(tx, newNode)
	if err != nil {
		return changeEntry{}, err
	}
	for _, rn := range revertedBlocks {
		ce.RevertedBlocks = append(ce.RevertedBlocks, rn.Block.ID())
	}
	for _, an := range appliedBlocks {
		ce.AppliedBlocks = append(ce.AppliedBlocks, an.Block.ID())
	}
	err = appendChangeLog(tx, ce)
	if err != nil {
		return changeEntry{}, err
	}
	return ce, nil
}

// threadedSleepOnFutureBlock will sleep until the timestamp of a future block
// has arrived.
//
// TODO: An attacker can broadcast a future block multiple times, resulting in a
// goroutine spinup for each future block.  Need to prevent that.
//
// TODO: An attacker could produce a very large number of future blocks,
// consuming memory. Need to prevent that.
func (cs *ConsensusSet) threadedSleepOnFutureBlock(b types.Block) {
	// Add this thread to the threadgroup.
	err := cs.tg.Add()
	if err != nil {
		return
	}
	defer cs.tg.Done()

	// Perform a soft-sleep while we wait for the block to become valid.
	select {
	case <-cs.tg.StopChan():
		return
	case <-time.After(time.Duration(b.Timestamp-(types.CurrentTimestamp()+types.FutureThreshold)) * time.Second):
		err := cs.managedAcceptBlocks([]types.Block{b})
		if err != nil {
			cs.log.Debugln("WARN: failed to accept a future block:", err)
		}
		cs.managedBroadcastBlock(b)
	}
}

// managedAcceptBlocks will try to add blocks to the consensus set. If the
// blocks do not extend the longest currently known chain, an error is
// returned but the blocks are still kept in memory. If the blocks extend a fork
// such that the fork becomes the longest currently known chain, the consensus
// set will reorganize itself to recognize the new longest fork. Accepted
// blocks are not relayed.
//
// Typically AcceptBlock should be used so that the accepted block is relayed.
// This method is typically only be used when there would otherwise be multiple
// consecutive calls to AcceptBlock with each successive call accepting the
// child block of the previous call.
func (cs *ConsensusSet) managedAcceptBlocks(blocks []types.Block) error {
	// Grab a lock on the consensus set.
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Make sure that blocks are consecutive. Though this isn't a strict
	// requirement, if blocks are not consecutive then it becomes a lot harder
	// to maintain correcetness when adding multiple blocks in a single tx.
	for i, b := range blocks {
		if i > 0 && b.Header().ParentID != blocks[i-1].ID() {
			return errNonLinearChain
		}
	}

	// Verify the headers for every block, throw out known blocks, and the
	// invalid blocks (which includes the children of invalid blocks).
	parents := make([]*processedBlock, 0, len(blocks))
	unknownBlocks := make([]types.Block, 0, len(blocks))
	headerError := cs.db.View(func(tx *bolt.Tx) error {
		// Do not accept a block if the database is inconsistent.
		if inconsistencyDetected(tx) {
			return errInconsistentSet
		}

		// Do some relatively inexpensive checks to validate the header and
		// block. Validation generally occurs in the order of least expensive
		// validation first. If an invalid block is found in the group, the
		// group will be trunacted.
		var err error
		var parent *processedBlock
		for i := 0; i < len(blocks); i++ {
			parent, err = cs.validateHeaderAndBlock(boltTxWrapper{tx}, blocks[i])
			if err == modules.ErrBlockKnown {
				// Skip over known blocks.
				err = nil
				continue
			}
			if err == errFutureTimestamp {
				// If a block exists in the future, we will hold the block in
				// memory until time to accept the block has arrived.
				go cs.threadedSleepOnFutureBlock(blocks[i])
			}
			if err != nil {
				break // Break the loop without appending the parent to the set of parents.
			}

			unknownBlocks = append(unknownBlocks, blocks[i])
			parents = append(parents, parent)
		}
		return err
	})

	// If there is an invalid header, then length of 'parents' will not match
	// the length of 'blocks'. Even if there was a header error, we want to add
	// every block that had a valid header, so we will go ahead with the update
	// call for those blocks.
	var nonExtending bool
	changes := make([]changeEntry, 0, len(unknownBlocks))
	blocksError := cs.db.Update(func(tx *bolt.Tx) error {
		// Add the blocks to the consensus set one at a time.
		for i := 0; i < len(unknownBlocks); i++ {
			// Submit the block for processing.
			changeEntry, err := cs.addBlockToTree(tx, unknownBlocks[i], parents[i])
			// We don't care about non-extending errors unless the last block
			// submitted is a non-extending block. This is handled by setting
			// 'nonExtending' to the most recent error result.
			nonExtending = err == modules.ErrNonExtendingBlock
			if nonExtending {
				err = nil
			}
			if err != nil {
				return err
			}

			// Sanity check - If reverted blocks is zero, applied blocks should also
			// be zero.
			if build.DEBUG && len(changeEntry.AppliedBlocks) == 0 && len(changeEntry.RevertedBlocks) != 0 {
				panic("after adding a change entry, there are no applied blocks but there are reverted blocks")
			}

			// Append to the set of changes.
			changes = append(changes, changeEntry)
		}
		return nil
	})
	// blocksError != nil means that the transaction was rolled back, and any
	// valid blocks which were discovered also got rolled back. Each change in
	// 'changes' corresponds to a valid block, those need to be added back in.
	if blocksError != nil {
		err := cs.db.Update(func(tx *bolt.Tx) error {
			for i := 0; i < len(changes); i++ {
				_, err := cs.addBlockToTree(tx, unknownBlocks[i], parents[i])
				if err != modules.ErrNonExtendingBlock && err != nil {
					return err
				}
			}
			return nil
		})
		// Something has gone wrong. Maybe the filesystem is having errors for
		// example. But under normal conditions, this code should not be
		// reached.
		if err != nil {
			return err
		}
	}

	// Stop here if the blocks did not extend the longest blockchain.
	if nonExtending {
		return modules.ErrNonExtendingBlock
	}

	// Update the subscribers with all of the consensus changes.
	for i := 0; i < len(changes); i++ {
		cs.readlockUpdateSubscribers(changes[i])
	}

	if blocksError != nil {
		return blocksError
	}
	if headerError != nil {
		return headerError
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
	err := cs.tg.Add()
	if err != nil {
		return err
	}
	defer cs.tg.Done()

	err = cs.managedAcceptBlocks([]types.Block{b})
	if err != nil {
		return err
	}
	cs.managedBroadcastBlock(b)
	return nil
}
