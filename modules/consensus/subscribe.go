package consensus

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"

	"github.com/NebulousLabs/bolt"
)

// computeConsensusChange computes the consensus change from the change entry
// at index 'i' in the change log. If i is out of bounds, an error is returned.
func (cs *ConsensusSet) computeConsensusChange(tx *bolt.Tx, ce changeEntry) (cc modules.ConsensusChange, err error) {
	cc.ID = ce.ID()
	for _, revertedBlockID := range ce.RevertedBlocks {
		revertedBlock, err := getBlockMap(tx, revertedBlockID)
		if build.DEBUG && err != nil {
			panic(err)
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
		if build.DEBUG && err != nil {
			panic(err)
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
	return
}

// readlockUpdateSubscribers will inform all subscribers of a new update to the
// consensus set. The call must be made with a demoted lock or a readlock.
// readlockUpdateSubscribers does not alter the changelog, the changelog must
// be updated beforehand.
func (cs *ConsensusSet) readlockUpdateSubscribers(ce changeEntry) {
	// Get the consensus change and send it to all subscribers.
	var cc modules.ConsensusChange
	err := cs.db.View(func(tx *bolt.Tx) error {
		// Compute the consensus change so it can be sent to subscribers.
		var err error
		cc, err = cs.computeConsensusChange(tx, ce)
		return err
	})
	if err != nil && build.DEBUG {
		panic(err)
	}
	for _, subscriber := range cs.subscribers {
		subscriber.ProcessConsensusChange(cc)
	}
}

// initializePersistentSubscribe will take a subscriber and feed them all of the
// consensus changes that have occurred since the change provided.
//
// As a special case, using an empty id as the start will have all the changes
// sent to the modules starting with the genesis block.
func (cs *ConsensusSet) initializePersistentSubscribe(subscriber modules.ConsensusSetSubscriber, start modules.ConsensusChangeID) error {
	return cs.db.View(func(tx *bolt.Tx) error {
		// 'exists' and 'entry' are going to be pointed to the first entry that
		// has not yet been seen by subscriber.
		var exists bool
		var entry changeEntry

		if start == (modules.ConsensusChangeID{}) {
			// Special case: if 'start' is blank, create an initial node
			// pointing to the genesis block. The subscriber will recieve the
			// diffs for all blocks in the consensus set, including the genesis
			// block.
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

		// Send all remaining consensus changes to the subscriber.
		for exists {
			cc, err := cs.computeConsensusChange(tx, entry)
			if err != nil {
				return err
			}
			subscriber.ProcessConsensusChange(cc)
			entry, exists = entry.NextEntry(tx)
		}
		return nil
	})
}

// ConsensusSetPersistentSubscribe adds a subscriber to the list of
// subscribers, and gives them every consensus change that has occured since
// the change with the provided id.
//
// As a special case, using an empty id as the start will have all the changes
// sent to the modules starting with the genesis block.
func (cs *ConsensusSet) ConsensusSetPersistentSubscribe(subscriber modules.ConsensusSetSubscriber, start modules.ConsensusChangeID) error {
	// Get the input module caught up to the currenct consnesus set.
	cs.mu.RLock()
	err := cs.initializePersistentSubscribe(subscriber, start)
	cs.mu.RUnlock()
	if err != nil {
		return err
	}

	// Only add the module as a subscriber if there was no error.
	cs.mu.Lock()
	cs.subscribers = append(cs.subscribers, subscriber)
	cs.mu.Unlock()
	return nil
}

// Unsubscribe removes a subscriber from the list of subscribers, allowing for
// garbage collection and rescanning. If the subscriber is not found in the
// subscriber database, no action is taken.
func (cs *ConsensusSet) Unsubscribe(subscriber modules.ConsensusSetSubscriber) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Search for the subscriber in the list of subscribers and remove it if
	// found.
	for i := range cs.subscribers {
		if cs.subscribers[i] == subscriber {
			cs.subscribers = append(cs.subscribers[0:i], cs.subscribers[i+1:]...)
			break
		}
	}
}
