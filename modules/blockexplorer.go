package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

type ExplorerInfo struct {
	CurrentBlock   types.Block       // The most recent block from consensus
	Target         types.Target      // The current target
	BlockchainSize types.BlockHeight // The height of the highest block
	CurrencyTotal  types.Currency    // The total amount of currency in circulation
	CurrencySent   types.Currency    // How much currency has been sent to other people
	CurrencySpent  types.Currency    // How much has been spent on file contracts
}

// The BlockExplorer interface provides access to the block explorer
type BlockExplorer interface {
	// A wrapper for the ConsensusSet CurrentBlock function. In
	// the future the blockExplorer will store its own version of
	// this block
	CurrentBlock() types.Block

	// Sends notifications when the module updates
	BlockExplorerNotify() <-chan struct{}

	// Function to populate and return an instance of the ExplorerInfo struct
}
