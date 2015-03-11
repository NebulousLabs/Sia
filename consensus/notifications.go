package consensus

// notifySubscribers sends an empty struct to every single remaining
// subscriber, indicating that the consensus set has changed.
func (s *State) notifySubscribers() {
	for _, subscriber := range s.subscriptions {
		// If the channel is already full, don't block.
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// SubscribeToConsensusChanges returns a channel that will be sent an empty
// struct every time the consensus set changes.
func (s *State) SubscribeToConsensusChanges() <-chan struct{} {
	counter := s.mu.Lock("state SubscribeToConsensusChanges")
	defer s.mu.Unlock("state SubscribeToConsensusChanges", counter)
	c := make(chan struct{}, 1)
	s.subscriptions = append(s.subscriptions, c)
	return c
}
