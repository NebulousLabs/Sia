package components

import (
// "github.com/NebulousLabs/Sia/consensus"
)

type RenterUpdate struct {
	HostDB HostDB
}

type Renter interface {
	// UpdateRenter changes the settings used by the host.
	UpdateRenter(RenterUpdate) error
}
