package consensus

import (
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
func (cs *ConsensusSet) threadedSendUpdates(update chan struct{}, subscriber modules.ConsensusSetSubscriber) {
	i := 0
	for {
		id := cs.mu.RLock()
		updateCount := len(cs.changeLog)
		cs.mu.RUnlock(id)
		for i < updateCount {
			// Build the consensus change that occured at the current index.
			var cc modules.ConsensusChange
			id := cs.mu.RLock()
			{
				for _, revertedBlockID := range cs.changeLog[i].revertedBlocks {
					revertedNode, exists := cs.blockMap[revertedBlockID]
					// Sanity check - node should exist.
					if build.DEBUG {
						if !exists {
							panic("grabbed a node that does not exist during a consensus change")
						}
					}

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
					appliedNode, exists := cs.blockMap[appliedBlockID]
					// Sanity check - node should exist.
					if build.DEBUG {
						if !exists {
							panic("grabbed a node that does not exist during a consensus change")
						}
					}

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
			}
			cs.mu.RUnlock(id)

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
func (cs *ConsensusSet) updateSubscribers(revertedNodes []*blockNode, appliedNodes []*blockNode) {
	// Sanity check - len(appliedNodes) should never be 0.
	if build.DEBUG {
		if len(appliedNodes) == 0 {
			panic("cannot have len(appliedNodes) = 0 in consensus set - blockchain must always get heavier")
		}
	}

	// Log the changes in the change log.
	var ce changeEntry
	for _, rn := range revertedNodes {
		ce.revertedBlocks = append(ce.revertedBlocks, rn.block.ID())
	}
	for _, an := range appliedNodes {
		ce.appliedBlocks = append(ce.appliedBlocks, an.block.ID())
	}
	cs.changeLog = append(cs.changeLog, ce)

	// Notify each update channel that a new update is ready.
	for _, subscriber := range cs.subscriptions {
		// If the channel is already full, don't block.
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// ConsensusSetNotify returns a channel that will be sent an empty struct every
// time the consensus set changes.
func (cs *ConsensusSet) ConsensusSetNotify() <-chan struct{} {
	c := make(chan struct{}, modules.NotifyBuffer)
	c <- struct{}{} // Notify subscriber about the genesis block.
	id := cs.mu.Lock()
	cs.subscriptions = append(cs.subscriptions, c)
	cs.mu.Unlock(id)
	return c
}

// ConsensusSetSubscribe accepts a new subscriber who will receive a call to
// ReceiveConsensusSetUpdate every time there is a change in the consensus set.
func (cs *ConsensusSet) ConsensusSetSubscribe(subscriber modules.ConsensusSetSubscriber) {
	c := make(chan struct{}, modules.NotifyBuffer)
	c <- struct{}{} // Notify subscriber about the genesis block.
	id := cs.mu.Lock()
	cs.subscriptions = append(cs.subscriptions, c)
	cs.mu.Unlock(id)
	go cs.threadedSendUpdates(c, subscriber)
}
