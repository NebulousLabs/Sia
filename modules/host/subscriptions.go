package host

// updateSubscribers will inform all subscribers of the new update to the host.
func (h *Host) updateSubscribers() {
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
	lockID := h.mu.Lock()
	c := make(chan struct{}, 1)
	h.subscriptions = append(h.subscriptions, c)
	h.mu.Unlock(lockID)
	return c
}
