package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	errChangeEntryNotFound = errors.New("module requesting a consensus starting point unknown to the database")
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
		cc, err = cs.computeConsensusChange(tx, cs.changeLog[len(cs.changeLog)-1])
		return err
	})
	if err != nil && build.DEBUG {
		panic(err)
	}
	for _, subscriber := range cs.subscribers {
		subscriber.ProcessConsensusChange(cc)
	}
}

// ConsensusChange returns the consensus change that occured at index 'i',
// returning an error if the input is out of bounds. For example,
// ConsensusChange(5) will return the 6th consensus change that was issued to
// subscribers. ConsensusChanges can be assumed to be consecutive.
func (cs *ConsensusSet) ConsensusChange(i int) (cc modules.ConsensusChange, err error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	err = cs.db.View(func(tx *bolt.Tx) error {
		cc, err = cs.computeConsensusChange(tx, cs.changeLog[i])
		return err
	})
	if err != nil {
		return modules.ConsensusChange{}, err
	}
	return cc, nil
}

// ConsensusSetSubscribe accepts a new subscriber who will receive a call to
// ProcessConsensusChange every time there is a change in the consensus set.
func (cs *ConsensusSet) ConsensusSetSubscribe(subscriber modules.ConsensusSetSubscriber) {
	cs.mu.Lock()
	cs.subscribers = append(cs.subscribers, subscriber)
	cs.mu.Demote()
	defer cs.mu.DemotedUnlock()

	err := cs.db.View(func(tx *bolt.Tx) error {
		for i := range cs.changeLog {
			cc, err := cs.computeConsensusChange(tx, cs.changeLog[i])
			if err != nil && build.DEBUG {
				panic(err)
			}
			subscriber.ProcessConsensusChange(cc)
		}
		return nil
	})
	if build.DEBUG && err != nil {
		panic(err)
	}
}

// ConsensusSetPersistentSubscribe adds a subscriber to the list of
// subscribers, and gives them every consensus change that has occured since
// the change with the provided id.
//
// As a special case, using an empty id as the start will have all the changes
// sent to the modules starting with the genesis block.
func (cs *ConsensusSet) ConsensusSetPersistentSubscribe(subscriber modules.ConsensusSetSubscriber, start modules.ConsensusChangeID) error {
	// Add the subscriber to the list of subscribers under lock, and then
	// demote while sending the subscriber all of the changes they've missed.
	cs.mu.Lock()
	cs.subscribers = append(cs.subscribers, subscriber)
	cs.mu.Demote()
	defer cs.mu.DemotedUnlock()

	err := cs.db.View(func(tx *bolt.Tx) error {
		var exists bool
		var entry changeEntry
		// Special case: if 'start' is blank, create an initial node pointing to
		// the genesis block.
		if start == (modules.ConsensusChangeID{}) {
			entry = cs.genesisEntry()
			exists = true
		} else {
			entry, exists = getEntry(tx, start)
			if !exists {
				return errChangeEntryNotFound
			}
			entry, exists = entry.NextEntry(tx)
		}

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
	if err != nil {
		return err
	}
	return nil
}
