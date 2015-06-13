package host

import (
	"github.com/NebulousLabs/Sia/modules"
)

// updateSubscribers will inform all subscribers of the new update to the host.
func (h *Host) threadedUpdateSubscribers() {
	lockID := h.mu.RLock()
	defer h.mu.RUnlock(lockID)

	for _, subscriber := range h.subscriptions {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// HostNotify returns a channel that will be sent a struct{} every time there
// is an update received from another module.
func (h *Host) HostNotify() <-chan struct{} {
	c := make(chan struct{}, modules.NotifyBuffer)
	lockID := h.mu.Lock()
	if h.consensusHeight > 0 {
		c <- struct{}{}
	}
	h.subscriptions = append(h.subscriptions, c)
	h.mu.Unlock(lockID)
	return c
}
