package modules

import (
	"github.com/NebulousLabs/Sia/types"
)

const (
	MiningPoolDir = "miningpool"
)

// The MiningPool interface provides functions that allow external miners
// mine for the pool
type MiningPool interface {
	// CreatePaymentChannel creates a payment channel from the MiningPool to the
	// miner. This allows for the pool to send currency to the miner off-chain
	// in order to prevent cluttering the network
	CreatePaymentChannel() error

	// SubmitBlockShare is called by the miner via a RPC. The miner submits a partial
	// block (one that meets an easier target set by the pool). The mining pool then
	// checks its validity and sends currency to the miner via the payment channel
	SubmitBlockShare(types.Block) error

	// Should there be some API call to communicate which side of a fork the pool is on?
}
