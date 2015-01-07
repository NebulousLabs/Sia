package components

import (
	"github.com/NebulousLabs/Sia/consensus"
)

// A ReadOnlyState is a state that can only be read, and is missing any write
// functions such as AcceptBlock and AcceptTransaction. This allows interfaces
// such as host to view the state without being able to modify anything.
type ReadOnlyState interface {
	// Height returns the number of blocks in the state.
	Height() consensus.BlockHeight

	// StorageProofSegmentIndex returns the segment index that should be used
	// in a storage proof given the window index and the contract id.
	StorageProofSegmentIndex(contractID consensus.ContractID, windowIndex consensus.BlockHeight) (index uint64, err error)
}
