package host

import (
// "github.com/NebulousLabs/Sia/modules"
)

type HostInfo struct {
	// Announcement modules.HostEntry

	StorageRemaining int
	ContractCount    int
}

func (h *Host) Info() (info HostInfo, err error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	info = HostInfo{
		// Announcement: h.announcement,

		StorageRemaining: int(h.spaceRemaining),
		ContractCount:    len(h.contracts),
	}
	return
}
