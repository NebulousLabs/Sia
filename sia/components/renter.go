package components

import (
// "github.com/NebulousLabs/Sia/consensus"
)

type RenterSettings struct {
	HostDB HostDB
}

type Renter interface {
	// UpdateRenterSettings changes the settings used by the host.
	UpdateRenterSettings(RenterSettings) error
}
