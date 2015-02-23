package consensus

// notifySubscribers sends an empty struct to every single remaining
// subscriber, indicating that the consensus set has changed.
func (s *State) notifySubscribers() {
	for _, sub := range s.subscriptions {
		// If the channel is already full, don't block.
		select {
		case sub <- struct{}{}:
		default:
		}
	}
}

// SubscribeToConsensusChanges returns a channel that will be sent an empty
// struct every time the consensus set changes.
func (s *State) SubscribeToConsensusChanges() (c chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c = make(chan struct{}, 1)
	s.subscriptions[s.subscriptionCounter] = c
	s.subscriptionCounter++
	return
}
