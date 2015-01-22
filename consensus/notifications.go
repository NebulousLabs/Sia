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
	alert = make(chan struct{})
	s.subscriptions = append(s.subscriptions, alert)
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
func (s *State) DiffsSince(id BlockID) (outputDiffs []OutputDiff, contractDiffs []ContractDiff, err error) {
	node, exists := s.blockMap[id]
	if !exists {
		err = errors.New("block is not known to the state or is an orphan")
		return
	}
	if !node.DiffsGenerated {
		err = errors.New("block is known to the state but has not had its diffs generated.")
		return
	}

	// Get all the diffs going backward, but reverse the new flag because they
	// are going backward. `reversedNodes` will contain a common parent whose
	// diffs should not be included, so we strip the last block provided.
	reversedNodes := s.backtrackToBlockchain(node)
	println("DOUBLE CHECK - first should be 1 larger than the second")
	println(len(reversedNodes))
	reversedNodes = reversedNodes[:len(reversedNodes)-1]
	println(len(reversedNodes))
	for _, node := range reversedNodes {
		for _, diff := range node.OutputDiffs {
			diff.New = !diff.New
			outputDiffs = append(outputDiffs, diff)
		}
		for _, diff := range node.ContractDiffs {
			diff.New = !diff.New
			contractDiffs = append(contractDiffs, diff)
		}
	}

	// Get all of the diffs going forward, starting from the height after the
	// pivot.
	height := reversedNodes[len(reversedNodes)-1].Height
	_, exists = s.currentPath[height]
	for exists {
		node := s.blockMap[s.currentPath[height]]
		for _, diff := range node.OutputDiffs {
			outputDiffs = append(outputDiffs, diff)
		}
		for _, diff := range node.ContractDiffs {
			contractDiffs = append(contractDiffs, diff)
		}

		height++
		_, exists = s.currentPath[height]
	}

	return
}
