package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// A changeEntry records a change to the consensus set that happened, and is
// used during subscriptions.
type changeEntry struct {
	revertedBlocks []types.BlockID
	appliedBlocks  []types.BlockID
}

// computeConsensusChange computes the consensus change from the change entry
// at index 'i' in the change log. If i is out of bounds, an error is returned.
func (cs *ConsensusSet) computeConsensusChange(i int) (cc modules.ConsensusChange, err error) {
	if i < 0 || i >= len(cs.changeLog) {
		err = errors.New("bounds error when querying changelog")
		return
	}

	for _, revertedBlockID := range cs.changeLog[i].revertedBlocks {
		revertedNode := cs.getBlockMapBn(revertedBlockID)

		// Because the direction is 'revert', the order of the diffs needs to
		// be flipped and the direction of the diffs also needs to be flipped.
		cc.RevertedBlocks = append(cc.RevertedBlocks, revertedNode.block)
		for i := len(revertedNode.siacoinOutputDiffs) - 1; i >= 0; i-- {
			scod := revertedNode.siacoinOutputDiffs[i]
			scod.Direction = !scod.Direction
			cc.SiacoinOutputDiffs = append(cc.SiacoinOutputDiffs, scod)
		}
		for i := len(revertedNode.fileContractDiffs) - 1; i >= 0; i-- {
			fcd := revertedNode.fileContractDiffs[i]
			fcd.Direction = !fcd.Direction
			cc.FileContractDiffs = append(cc.FileContractDiffs, fcd)
		}
		for i := len(revertedNode.siafundOutputDiffs) - 1; i >= 0; i-- {
			sfod := revertedNode.siafundOutputDiffs[i]
			sfod.Direction = !sfod.Direction
			cc.SiafundOutputDiffs = append(cc.SiafundOutputDiffs, sfod)
		}
		for i := len(revertedNode.delayedSiacoinOutputDiffs) - 1; i >= 0; i-- {
			dscod := revertedNode.delayedSiacoinOutputDiffs[i]
			dscod.Direction = !dscod.Direction
			cc.DelayedSiacoinOutputDiffs = append(cc.DelayedSiacoinOutputDiffs, dscod)
		}
		for i := len(revertedNode.siafundPoolDiffs) - 1; i >= 0; i-- {
			sfpd := revertedNode.siafundPoolDiffs[i]
			sfpd.Direction = modules.DiffRevert
			cc.SiafundPoolDiffs = append(cc.SiafundPoolDiffs, sfpd)
		}
	}
	for _, appliedBlockID := range cs.changeLog[i].appliedBlocks {
		appliedNode := cs.getBlockMapBn(appliedBlockID)

		cc.AppliedBlocks = append(cc.AppliedBlocks, appliedNode.block)
		for _, scod := range appliedNode.siacoinOutputDiffs {
			cc.SiacoinOutputDiffs = append(cc.SiacoinOutputDiffs, scod)
		}
		for _, fcd := range appliedNode.fileContractDiffs {
			cc.FileContractDiffs = append(cc.FileContractDiffs, fcd)
		}
		for _, sfod := range appliedNode.siafundOutputDiffs {
			cc.SiafundOutputDiffs = append(cc.SiafundOutputDiffs, sfod)
		}
		for _, dscod := range appliedNode.delayedSiacoinOutputDiffs {
			cc.DelayedSiacoinOutputDiffs = append(cc.DelayedSiacoinOutputDiffs, dscod)
		}
		for _, sfpd := range appliedNode.siafundPoolDiffs {
			cc.SiafundPoolDiffs = append(cc.SiafundPoolDiffs, sfpd)
		}
	}
	return
}

// updateSubscribers will inform all subscribers of the new update to the
// consensus set.
func (cs *ConsensusSet) updateSubscribers(revertedNodes []*processedBlock, appliedNodes []*processedBlock) {
	// Log the changes in the change log.
	var ce changeEntry
	for _, rn := range revertedNodes {
		ce.revertedBlocks = append(ce.revertedBlocks, rn.Block.ID())
	}
	for _, an := range appliedNodes {
		ce.appliedBlocks = append(ce.appliedBlocks, an.Block.ID())
	}
	cs.changeLog = append(cs.changeLog, ce)

	// Notify each update channel that a new update is ready.
	cc, err := cs.computeConsensusChange(len(cs.changeLog) - 1)
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
func (cs *ConsensusSet) ConsensusChange(i int) (modules.ConsensusChange, error) {
	id := cs.mu.RLock()
	defer cs.mu.RUnlock(id)
	return cs.computeConsensusChange(i)
}

// ConsensusSetSubscribe accepts a new subscriber who will receive a call to
// ProcessConsensusChange every time there is a change in the consensus set.
func (cs *ConsensusSet) ConsensusSetSubscribe(subscriber modules.ConsensusSetSubscriber) {
	id := cs.mu.Lock()
	cs.subscribers = append(cs.subscribers, subscriber)
	for i := range cs.changeLog {
		cc, err := cs.computeConsensusChange(i)
		if err != nil && build.DEBUG {
			panic(err)
		}
		subscriber.ProcessConsensusChange(cc)
	}
	cs.mu.Unlock(id)
}
