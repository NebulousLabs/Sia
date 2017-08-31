package consensus

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"

	siasync "github.com/NebulousLabs/Sia/sync"
	"github.com/NebulousLabs/bolt"
)

// computeConsensusChange computes the consensus change from the change entry
// at index 'i' in the change log. If i is out of bounds, an error is returned.
func (cs *ConsensusSet) computeConsensusChange(tx *bolt.Tx, ce changeEntry) (modules.ConsensusChange, error) {
	cc := modules.ConsensusChange{
		ID: ce.ID(),
	}
	for _, revertedBlockID := range ce.RevertedBlocks {
		revertedBlock, err := getBlockMap(tx, revertedBlockID)
		if err != nil {
			cs.log.Critical("getBlockMap failed in computeConsensusChange:", err)
			return modules.ConsensusChange{}, err
		}

		// Because the direction is 'revert', the order of the diffs needs to
		// be flipped and the direction of the diffs also needs to be flipped.
		cc.RevertedBlocks = append(cc.RevertedBlocks, revertedBlock.Block)
		for i := len(revertedBlock.SiacoinOutputDiffs) - 1; i >= 0; i-- {
			scod := revertedBlock.SiacoinOutputDiffs[i]
			scod.Direction = !scod.Direction
			cc.SiacoinOutputDiffs = append(cc.SiacoinOutputDiffs, scod)
		}
		for i := len(revertedBlock.FileContractDiffs) - 1; i >= 0; i-- {
			fcd := revertedBlock.FileContractDiffs[i]
			fcd.Direction = !fcd.Direction
			cc.FileContractDiffs = append(cc.FileContractDiffs, fcd)
		}
		for i := len(revertedBlock.SiafundOutputDiffs) - 1; i >= 0; i-- {
			sfod := revertedBlock.SiafundOutputDiffs[i]
			sfod.Direction = !sfod.Direction
			cc.SiafundOutputDiffs = append(cc.SiafundOutputDiffs, sfod)
		}
		for i := len(revertedBlock.DelayedSiacoinOutputDiffs) - 1; i >= 0; i-- {
			dscod := revertedBlock.DelayedSiacoinOutputDiffs[i]
			dscod.Direction = !dscod.Direction
			cc.DelayedSiacoinOutputDiffs = append(cc.DelayedSiacoinOutputDiffs, dscod)
		}
		for i := len(revertedBlock.SiafundPoolDiffs) - 1; i >= 0; i-- {
			sfpd := revertedBlock.SiafundPoolDiffs[i]
			sfpd.Direction = modules.DiffRevert
			cc.SiafundPoolDiffs = append(cc.SiafundPoolDiffs, sfpd)
		}
	}
	for _, appliedBlockID := range ce.AppliedBlocks {
		appliedBlock, err := getBlockMap(tx, appliedBlockID)
		if err != nil {
			cs.log.Critical("getBlockMap failed in computeConsensusChange:", err)
			return modules.ConsensusChange{}, err
		}

		cc.AppliedBlocks = append(cc.AppliedBlocks, appliedBlock.Block)
		for _, scod := range appliedBlock.SiacoinOutputDiffs {
			cc.SiacoinOutputDiffs = append(cc.SiacoinOutputDiffs, scod)
		}
		for _, fcd := range appliedBlock.FileContractDiffs {
			cc.FileContractDiffs = append(cc.FileContractDiffs, fcd)
		}
		for _, sfod := range appliedBlock.SiafundOutputDiffs {
			cc.SiafundOutputDiffs = append(cc.SiafundOutputDiffs, sfod)
		}
		for _, dscod := range appliedBlock.DelayedSiacoinOutputDiffs {
			cc.DelayedSiacoinOutputDiffs = append(cc.DelayedSiacoinOutputDiffs, dscod)
		}
		for _, sfpd := range appliedBlock.SiafundPoolDiffs {
			cc.SiafundPoolDiffs = append(cc.SiafundPoolDiffs, sfpd)
		}
	}

	// Grab the child target and the minimum valid child timestamp.
	recentBlock := ce.AppliedBlocks[len(ce.AppliedBlocks)-1]
	pb, err := getBlockMap(tx, recentBlock)
	if err != nil {
		cs.log.Critical("could not find process block for known block")
	}
	cc.ChildTarget = pb.ChildTarget
	cc.MinimumValidChildTimestamp = cs.blockRuleHelper.minimumValidChildTimestamp(tx.Bucket(BlockMap), pb)

	currentBlock := currentBlockID(tx)
	if cs.synced && recentBlock == currentBlock {
		cc.Synced = true
	}

	// Add the unexported tryTransactionSet function.
	cc.TryTransactionSet = cs.tryTransactionSet

	return cc, nil
}

// readLockUpdateSubscribers will inform all subscribers of a new update to the
// consensus set. updateSubscribers does not alter the changelog, the changelog
// must be updated beforehand.
func (cs *ConsensusSet) updateSubscribers(ce changeEntry) {
	// Get the consensus change and send it to all subscribers.
	var cc modules.ConsensusChange
	err := cs.db.View(func(tx *bolt.Tx) error {
		// Compute the consensus change so it can be sent to subscribers.
		var err error
		cc, err = cs.computeConsensusChange(tx, ce)
		return err
	})
	if err != nil {
		cs.log.Critical("computeConsensusChange failed:", err)
		return
	}
	for _, subscriber := range cs.subscribers {
		subscriber.ProcessConsensusChange(cc)
	}
}

// managedInitializeSubscribe will take a subscriber and feed them all of the
// consensus changes that have occurred since the change provided.
//
// As a special case, using an empty id as the start will have all the changes
// sent to the modules starting with the genesis block.
func (cs *ConsensusSet) managedInitializeSubscribe(subscriber modules.ConsensusSetSubscriber, start modules.ConsensusChangeID,
	cancel <-chan struct{}) error {

	if start == modules.ConsensusChangeRecent {
		return nil
	}

	// 'exists' and 'entry' are going to be pointed to the first entry that
	// has not yet been seen by subscriber.
	var exists bool
	var entry changeEntry

	cs.mu.RLock()
	err := cs.db.View(func(tx *bolt.Tx) error {
		if start == modules.ConsensusChangeBeginning {
			// Special case: for modules.ConsensusChangeBeginning, create an
			// initial node pointing to the genesis block. The subscriber will
			// receive the diffs for all blocks in the consensus set, including
			// the genesis block.
			entry = cs.genesisEntry()
			exists = true
		} else {
			// The subscriber has provided an existing consensus change.
			// Because the subscriber already has this consensus change,
			// 'entry' and 'exists' need to be pointed at the next consensus
			// change.
			entry, exists = getEntry(tx, start)
			if !exists {
				// modules.ErrInvalidConsensusChangeID is a named error that
				// signals a break in synchronization between the consensus set
				// persistence and the subscriber persistence. Typically,
				// receiving this error means that the subscriber needs to
				// perform a rescan of the consensus set.
				return modules.ErrInvalidConsensusChangeID
			}
			entry, exists = entry.NextEntry(tx)
		}
		return nil
	})
	cs.mu.RUnlock()
	if err != nil {
		return err
	}

	// Send all remaining consensus changes to the subscriber.
	for exists {
		// Send changes in batches of 100 so that we don't hold the
		// lock for too long.
		cs.mu.RLock()
		err = cs.db.View(func(tx *bolt.Tx) error {
			for i := 0; i < 100 && exists; i++ {
				select {
				case <-cancel:
					return siasync.ErrStopped
				default:
				}
				cc, err := cs.computeConsensusChange(tx, entry)
				if err != nil {
					return err
				}
				subscriber.ProcessConsensusChange(cc)
				entry, exists = entry.NextEntry(tx)
			}
			return nil
		})
		cs.mu.RUnlock()
		if err != nil {
			return err
		}
		// Flush DB pages from memory. Caching the pages doesn't improve
		// performance much anyway, since they are only read once.
		cs.mu.Lock()
		err = cs.db.Update(func(tx *bolt.Tx) error {
			return tx.FlushDBPages()
		})
		cs.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

// ConsensusSetSubscribe adds a subscriber to the list of subscribers, and
// gives them every consensus change that has occurred since the change with
// the provided id.
//
// As a special case, using an empty id as the start will have all the changes
// sent to the modules starting with the genesis block.
func (cs *ConsensusSet) ConsensusSetSubscribe(subscriber modules.ConsensusSetSubscriber, start modules.ConsensusChangeID,
	cancel <-chan struct{}) error {

	err := cs.tg.Add()
	if err != nil {
		return err
	}
	defer cs.tg.Done()

	// Get the input module caught up to the current consensus set.
	err = cs.managedInitializeSubscribe(subscriber, start, cancel)
	if err != nil {
		return err
	}

	// Add the module to the list of subscribers.
	cs.mu.Lock()
	// Sanity check - subscriber should not be already subscribed.
	for _, s := range cs.subscribers {
		if s == subscriber {
			build.Critical("refusing to double-subscribe subscriber")
		}
	}
	cs.subscribers = append(cs.subscribers, subscriber)
	cs.mu.Unlock()
	return nil
}

// Unsubscribe removes a subscriber from the list of subscribers, allowing for
// garbage collection and rescanning. If the subscriber is not found in the
// subscriber database, no action is taken.
func (cs *ConsensusSet) Unsubscribe(subscriber modules.ConsensusSetSubscriber) {
	if cs.tg.Add() != nil {
		return
	}
	defer cs.tg.Done()
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Search for the subscriber in the list of subscribers and remove it if
	// found.
	for i := range cs.subscribers {
		if cs.subscribers[i] == subscriber {
			// nil the subscriber entry (otherwise it will not be GC'd if it's
			// at the end of the subscribers slice).
			cs.subscribers[i] = nil
			// Delete the entry from the slice.
			cs.subscribers = append(cs.subscribers[0:i], cs.subscribers[i+1:]...)
			break
		}
	}
}
