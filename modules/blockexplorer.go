package modules

import (
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

const (
	ExplorerDir = "explorer"
)

// Used for the BlockInfo call
type ExplorerBlockData struct {
	ID        types.BlockID   // The id hash of the block
	Timestamp types.Timestamp // The timestamp on the block
	Target    types.Target    // The target the block was mined for
	Size      uint64          // The size in bytes of the marshalled block
}

type ExplorerStatus struct {
	Height              types.BlockHeight
	Block               types.Block
	Target              types.Target
	TotalCurrency       types.Currency
	ActiveContractCount uint64
	ActiveContractCosts types.Currency
	ActiveContractSize  uint64
	TotalContractCount  uint64
	TotalContractCosts  types.Currency
	TotalContractSize   uint64
}

// The BlockExplorer interface provides access to the block explorer
type BlockExplorer interface {
	// Returns a slice of data points about blocks. Called
	// primarly by the blockdata api call
	BlockInfo(types.BlockHeight, types.BlockHeight) ([]ExplorerBlockData, error)

	// Function to return status of a bunch of static variables,
	// in the form of an ExplorerStatus struct
	ExplorerStatus() ExplorerStatus

	// Function to safely shut down the block explorer. Closes the database
	Close() error

	// Sends notifications when the module updates
	BlockExplorerNotify() <-chan struct{}

	// Returns information pertaining to a given hash
	GetHashInfo(crypto.Hash) (interface{}, error)
}
