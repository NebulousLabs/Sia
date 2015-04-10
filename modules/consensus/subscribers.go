package consensus

import (
	"github.com/NebulousLabs/Sia/types"
)

// A ConsensusSetSubscriber is an object that receives updates to the consensus
// set every time there is a change in consensus.
type ConsensusSetSubscriber interface {
	// ReceiveConsensusSetUpdate sends a consensus update to a module through a
	// function call. Updates will always be sent in the correct order.
	// Usually, the function receiving the updates will also process the
	// changes. If the function blocks indefinitely, the state will still
	// function.
	ReceiveConsensusSetUpdate(revertedBlocks []types.Block, appliedBlocks []types.Block)
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
func (s *State) threadedSendUpdates(update chan struct{}, subscriber ConsensusSetSubscriber) {
	i := 0
	for {
		id := s.mu.RLock()
		updateCount := len(s.revertUpdates)
		s.mu.RUnlock(id)
		for i < updateCount {
			// Get the set of blocks that changed since the previous update.
			id := s.mu.RLock()
			var revertedBlocks, appliedBlocks []types.Block
			for _, node := range s.revertUpdates[i] {
				revertedBlocks = append(revertedBlocks, node.block)
			}
			for _, node := range s.applyUpdates[i] {
				appliedBlocks = append(appliedBlocks, node.block)
			}
			s.mu.RUnlock(id)

			// Update the subscriber with the changes.
			subscriber.ReceiveConsensusSetUpdate(revertedBlocks, appliedBlocks)
			i++
		}

		// Wait until there has been another update.
		<-update
	}
}

// updateSubscribers calls ReceiveConsensusSetUpdate on all of the subscribers
// to the consensus set.
func (s *State) updateSubscribers(revertedNodes []*blockNode, appliedNodes []*blockNode) {
	// Add the changes to the change set.
	s.revertUpdates = append(s.revertUpdates, revertedNodes)
	s.applyUpdates = append(s.applyUpdates, appliedNodes)

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
	id := s.mu.Lock()
	c := make(chan struct{}, 1)
	s.subscriptions = append(s.subscriptions, c)
	s.mu.Unlock(id)
	return c
}

// ConsensusSetSubscribe accepts a new subscriber who will receive a call to
// ReceiveConsensusSetUpdate every time there is a change in the consensus set.
func (s *State) ConsensusSetSubscribe(subscriber ConsensusSetSubscriber) {
	c := make(chan struct{}, 1)
	id := s.mu.Lock()
	s.subscriptions = append(s.subscriptions, c)
	s.mu.Unlock(id)
	go s.threadedSendUpdates(c, subscriber)
}
