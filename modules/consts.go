package modules

import "github.com/NebulousLabs/Sia/types"

// Consts that are required by multiple modules
const (
	// maxTxnAge determines the maximum age of a transaction (in block height)
	// allowed before the transaction is pruned from the transaction pool.
	MaxTxnAge = types.BlockHeight(24)
)
