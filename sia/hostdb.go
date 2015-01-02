package sia

import (
	"github.com/NebulousLabs/Sia/consensus"
)

type HostDB interface {
	// Info returns an arbitrary byte slice presumably with information about
	// the status of the hostdb. Info is not relevant to the sia package, but
	// instead toa frontend.
	Info() ([]byte, error)

	// Update gives the hostdb a set of blocks that have been applied and
	// reversed.
	Update(initialStateHeight consensus.BlockHeight, rewoundBlocks []consensus.Block, appliedBlocks []consensus.Block) error
}
