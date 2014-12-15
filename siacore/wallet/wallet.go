package wallet

import (
	"github.com/NebulousLabs/Andromeda/consensus"
	"github.com/NebulousLabs/Andromeda/signatures"
)

// Wallet is the struct used in the wallet package. Though it seems like
// stuttering, most users are going to be calling functions like `w :=
// wallet.New()`
type Wallet struct {
	SecretKey       signatures.SecretKey
	SpendConditions consensus.SpendConditions

	Balance consensus.Currency
	OwnedOutputs map[consensus.OutputID]struct{}
	SpendOutputs map[consensus.OutputID]struct{}
}
