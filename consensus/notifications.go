package consensus

import (
	"errors"
)

// Subscribe allows a module to subscribe to the state, which means that it'll
// receive a notification (in the form of an empty struct) each time the state
// gets a new block.
func (s *State) Subscribe() (alert chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	alert = make(chan struct{}, 1)
	s.subscriptions = append(s.subscriptions, alert)
	alert <- struct{}{}
	return
}

// notifySubscribers sends a ConsensusChange notification to every subscriber
//
// The sending is done in a separate goroutine to prevent deadlock if one
// subscriber becomes unresponsive.
func (s *State) notifySubscribers() {
	for _, sub := range s.subscriptions {
		select {
		case sub <- struct{}{}:
			// Receiver has been notified of an update.
		default:
			// Receiver already has notification to check for updates.
		}
	}
}

// OutputDiffsSince returns a set of output diffs representing how the state
// has changed since block `id`. OutputDiffsSince will flip the `new` value for
// diffs that got reversed.
func (s *State) BlocksSince(id BlockID) (removedBlocks, addedBlocks []BlockID, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, exists := s.blockMap[id]
	if !exists {
		err = errors.New("block is not known to the state or is an orphan")
		return
	}

	// Get all the ids from going backwards to the blockchain.
	reversedNodes := s.backtrackToBlockchain(node)
	// Eliminate the last node, which is the pivot node, whose diffs are already
	// known.
	reversedNodes = reversedNodes[:len(reversedNodes)-1]
	for _, reversedNode := range reversedNodes {
		removedBlocks = append(removedBlocks, reversedNode.Block.ID())
	}

	// Get all the ids going forward from the pivot node.
	height := reversedNodes[len(reversedNodes)-1].Height
	for _, exists := s.currentPath[height]; exists; height++ {
		node := s.blockMap[s.currentPath[height]]
		addedBlocks = append(addedBlocks, node.Block.ID())
		_, exists = s.currentPath[height]
	}

	return
}
