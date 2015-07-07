package consensus

import (
	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

// threadedSendUpdates sends updates to a specific subscriber as they become
// available. One thread is needed per subscriber. A separate function was
// needed due to race conditions; subscribers must receive updates in the
// correct order. Furthermore, a deadlocked subscriber should not interfere
// with consensus; updates cannot make blocking calls from any thread that is
// holding a lock on consensus. The result is a construction where all updates
// are added to a list of updates in the consensus set while the consensus set
// is locked. Then, a separate thread for each subscriber will be notified (via
// the update chan) that there are new updates. The thread will lock the
// consensus set for long enough to get the updates, and then will unlock the
// consensus set while it makes a blocking call to the subscriber. If the
// subscriber deadlocks or has problems, the thread will stall indefinitely,
// but the rest of consensus will not be disrupted.
func (s *State) threadedSendUpdates(update chan struct{}, subscriber modules.ConsensusSetSubscriber) {
	i := 0
	for {
		id := s.mu.RLock()
		updateCount := len(s.consensusChanges)
		s.mu.RUnlock(id)
		for i < updateCount {
			// Get the set of blocks that changed since the previous update.
			id := s.mu.RLock()
			cc := s.consensusChanges[i]
			s.mu.RUnlock(id)

			// Update the subscriber with the changes.
			subscriber.ReceiveConsensusSetUpdate(cc)
			i++
		}

		// Wait until there has been another update.
		<-update
	}
}

// updateSubscribers will inform all subscribers of the new update to the
// consensus set.
func (s *State) updateSubscribers(revertedNodes []*blockNode, appliedNodes []*blockNode) {
	// Sanity check - len(appliedNodes) should never be 0.
	if build.DEBUG {
		if len(appliedNodes) == 0 {
			panic("cannot have len(appliedNodes) = 0 in consensus set - blockchain must always get heavier")
		}
	}

	// Take the nodes and condense them into a consensusChange object.
	var cc modules.ConsensusChange
	for _, rn := range revertedNodes {
		// Because the direction is 'revert', the order of the diffs needs to
		// be flipped and the direction of the diffs also needs to be flipped.
		cc.RevertedBlocks = append(cc.RevertedBlocks, rn.block)
		for i := len(rn.siacoinOutputDiffs) - 1; i >= 0; i-- {
			scod := rn.siacoinOutputDiffs[i]
			scod.Direction = !scod.Direction
			cc.SiacoinOutputDiffs = append(cc.SiacoinOutputDiffs, scod)
		}
		for i := len(rn.fileContractDiffs) - 1; i >= 0; i-- {
			fcd := rn.fileContractDiffs[i]
			fcd.Direction = !fcd.Direction
			cc.FileContractDiffs = append(cc.FileContractDiffs, fcd)
		}
		for i := len(rn.siafundOutputDiffs) - 1; i >= 0; i-- {
			sfod := rn.siafundOutputDiffs[i]
			sfod.Direction = !sfod.Direction
			cc.SiafundOutputDiffs = append(cc.SiafundOutputDiffs, sfod)
		}
		for i := len(rn.delayedSiacoinOutputDiffs) - 1; i >= 0; i-- {
			dscod := rn.delayedSiacoinOutputDiffs[i]
			dscod.Direction = !dscod.Direction
			cc.DelayedSiacoinOutputDiffs = append(cc.DelayedSiacoinOutputDiffs, dscod)
		}
		for i := len(rn.siafundPoolDiffs) - 1; i >= 0; i-- {
			sfpd := rn.siafundPoolDiffs[i]
			sfpd.Direction = modules.DiffRevert
			cc.SiafundPoolDiffs = append(cc.SiafundPoolDiffs, sfpd)
		}
	}
	for _, an := range appliedNodes {
		cc.AppliedBlocks = append(cc.AppliedBlocks, an.block)
		for _, scod := range an.siacoinOutputDiffs {
			cc.SiacoinOutputDiffs = append(cc.SiacoinOutputDiffs, scod)
		}
		for _, fcd := range an.fileContractDiffs {
			cc.FileContractDiffs = append(cc.FileContractDiffs, fcd)
		}
		for _, sfod := range an.siafundOutputDiffs {
			cc.SiafundOutputDiffs = append(cc.SiafundOutputDiffs, sfod)
		}
		for _, dscod := range an.delayedSiacoinOutputDiffs {
			cc.DelayedSiacoinOutputDiffs = append(cc.DelayedSiacoinOutputDiffs, dscod)
		}
		for _, sfpd := range an.siafundPoolDiffs {
			cc.SiafundPoolDiffs = append(cc.SiafundPoolDiffs, sfpd)
		}
	}
	// Add the changes to the change set.
	s.consensusChanges = append(s.consensusChanges, cc)

	// Notify each update channel that a new update is ready.
	for _, subscriber := range s.subscriptions {
		// If the channel is already full, don't block.
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// ConsensusSetNotify returns a channel that will be sent an empty struct every
// time the consensus set changes.
func (s *State) ConsensusSetNotify() <-chan struct{} {
	c := make(chan struct{}, modules.NotifyBuffer)
	c <- struct{}{} // Notify subscriber about the genesis block.
	id := s.mu.Lock()
	s.subscriptions = append(s.subscriptions, c)
	s.mu.Unlock(id)
	return c
}

// ConsensusSetSubscribe accepts a new subscriber who will receive a call to
// ReceiveConsensusSetUpdate every time there is a change in the consensus set.
func (s *State) ConsensusSetSubscribe(subscriber modules.ConsensusSetSubscriber) {
	c := make(chan struct{}, modules.NotifyBuffer)
	c <- struct{}{} // Notify subscriber about the genesis block.
	id := s.mu.Lock()
	s.subscriptions = append(s.subscriptions, c)
	s.mu.Unlock(id)
	go s.threadedSendUpdates(c, subscriber)
}
