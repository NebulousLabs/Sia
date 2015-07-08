package blockexplorer

import (
	"github.com/NebulousLabs/Sia/modules"
)

// updateSubscribers will inform subscribers of any updates to the block explorer
func (be *BlockExplorer) updateSubscribers() {
	for _, subscriber := range be.subscriptions {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

// BlockExplorerNotify returns a channel that will be sent an empty
// struct every time there is an update recieved from from another module
func (be *BlockExplorer) BlockExplorerNotify() <-chan struct{} {
	c := make(chan struct{}, modules.NotifyBuffer)
	lockID := be.mu.Lock()
	c <- struct{}{}
	be.subscriptions = append(be.subscriptions, c)
	be.mu.Unlock(lockID)
	return c
}
