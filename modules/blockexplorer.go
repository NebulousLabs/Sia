package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

// Used for the BlockInfo call
type BlockData struct {
	Timestamp types.Timestamp // The timestamp on the block
	Target    types.Target    // The target the block was mined for
	Size      uint64          // The size in bytes of the marshalled block
}

// Used for the CurrentBlock call
type CurrentBlockData struct {
	Block  types.Block
	Target types.Target
}

// Used for SiaCoins call
type SiacoinData struct {
	CurrencySent  types.Currency
	TotalCurrency types.Currency
}

type FileContractData struct {
	FileContractCount uint64
	FileContractCosts types.Currency
}

// The BlockExplorer interface provides access to the block explorer
type BlockExplorer interface {
	// Returns a slice of data points about blocks. Called
	// primarly by the blockdata api call
	BlockInfo(types.BlockHeight, types.BlockHeight) ([]BlockData, error)

	// Returns the current hegiht of the blockchain
	BlockHeight() types.BlockHeight

	// CurrentBlock returns the current block and target
	CurrentBlock() CurrentBlockData

	// SiaCoins retuns high level data about the siacoins in circulation
	Siacoins() SiacoinData

	// FileContracts
	FileContracts() FileContractData

	// Sends notifications when the module updates
	BlockExplorerNotify() <-chan struct{}
}
