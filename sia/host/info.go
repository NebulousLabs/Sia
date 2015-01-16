package host

import (
	"github.com/NebulousLabs/Sia/sia/components"
)

func (h *Host) HostInfo() (info components.HostInfo, err error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	info = components.HostInfo{
		Announcement: h.announcement,

		StorageRemaining: int(h.spaceRemaining),
		ContractCount:    len(h.contracts),
	}
	return
}
