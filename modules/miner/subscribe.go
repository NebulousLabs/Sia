package miner

import (
	"github.com/NebulousLabs/Sia/modules"
)

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

// MinerNotify adds a subscriber to the miner.
func (m *Miner) MinerNotify() <-chan struct{} {
	c := make(chan struct{}, modules.NotifyBuffer)
	m.mu.Lock()
	if m.height > 0 {
		c <- struct{}{}
	}
	m.subscribers = append(m.subscribers, c)
	m.mu.Unlock()
	return c
}
