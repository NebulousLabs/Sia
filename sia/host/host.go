package host

import (
	"github.com/NebulousLabs/Sia/sia/hostdb"
)

type Host interface {
	// UpdateSettings changes the settings used by the host.
	UpdateSettings(hostdb.HostAnnouncement) error
}
