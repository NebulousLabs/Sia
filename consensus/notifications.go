package consensus

// notifySubscribers sends a ConsensusChange notification to every subscriber
//
// The sending is done in a separate goroutine to prevent deadlock if one
// subscriber becomes unresponsive.
//
// TODO: What happens if a subscriber stops pulling info from their channel. If
// they don't close the channel but stop pulling out elements, will the system
// lock up? If something stops responding suddenly, there needs to be a way to
// keep going, the state just deletes, closes, or ignores the channel or
// something. Perhaps the state will close the channel if the buffer fills up,
// assuming that the component has shut down unexpectedly. If the component was
// just being slow, it can do some catching up and re-subscribe. If we do end
// up closing subscription channels then we should switch from a slice to a
// map for s.consensusSubscriptions.
//
// TODO: This seems to be causing deadlock.
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
