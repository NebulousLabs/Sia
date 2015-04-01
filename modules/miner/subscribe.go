package miner

// notifySubscribers tells each subscriber that the miner has received an
// update.
func (m *Miner) notifySubscribers() {
	for _, subscriber := range m.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// MinerSubscribe adds a subscriber to the miner.
func (m *Miner) MinerSubscribe() <-chan struct{} {
	c := make(chan struct{}, 1)
	m.mu.Lock()
	m.subscribers = append(m.subscribers, c)
	m.mu.Unlock()
	return c
}
