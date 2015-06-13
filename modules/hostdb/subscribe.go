package hostdb

import (
	"github.com/NebulousLabs/Sia/modules"
)

// notifySubscribers tells each subscriber that the hostdb has received an
// update.
func (hdb *HostDB) notifySubscribers() {
	for _, subscriber := range hdb.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// HostDBNotify adds a subscriber to the hostdb.
func (hdb *HostDB) HostDBNotify() <-chan struct{} {
	c := make(chan struct{}, modules.NotifyBuffer)
	id := hdb.mu.Lock()
	if hdb.consensusHeight > 0 {
		c <- struct{}{}
	}
	hdb.subscribers = append(hdb.subscribers, c)
	hdb.mu.Unlock(id)
	return c
}
