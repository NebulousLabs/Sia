package hostdb

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// The HostDB interface actually uses a struct from the hostdb subpackage,
// which is a bit of a bad practice. The alternative would be to create a
// different package to manage things like host entries and host announcements,
// but I think it makes enough sense to define them in the same package that
// also provides an example (and the primary) implementation of the hostdb
// interface.
//
// Maybe though we can make the HostAnnouncement and the HostEntry their own
// interfaces.

type HostDB interface {
	// Info returns an arbitrary byte slice presumably with information about
	// the status of the hostdb. Info is not relevant to the sia package, but
	// instead toa frontend.
	Info() ([]byte, error)

	// Update gives the hostdb a set of blocks that have been applied and
	// reversed.
	Update(initialStateHeight consensus.BlockHeight, rewoundBlocks []consensus.Block, appliedBlocks []consensus.Block) error

	// Insert puts a host entry into the host database.
	Insert(HostEntry) error

	// Remove pulls a host entry from the host database.
	Remove(id string) error

	// RandomHost pulls a host entry at random from the database, weighted
	// according to whatever score is assigned the hosts.
	RandomHost() (HostEntry, error)
}
