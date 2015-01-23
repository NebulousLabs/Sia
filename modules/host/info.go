package host

type HostInfo struct {
	Announcement HostAnnouncement

	StorageRemaining int
	ContractCount    int
}

func (h *Host) Info() (info HostInfo, err error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	info = components.HostInfo{
		Announcement: h.announcement,

		StorageRemaining: int(h.spaceRemaining),
		ContractCount:    len(h.contracts),
	}
	return
}
