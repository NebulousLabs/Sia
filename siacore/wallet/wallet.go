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

	Balance      consensus.Currency
	OwnedOutputs map[consensus.OutputID]struct{}
	SpentOutputs map[consensus.OutputID]struct{}
}

// New creates an initializes a Wallet.
func New() (w *Wallet, err error) {
	sk, pk, err := signatures.GenerateKeyPair()
	if err != nil {
		return
	}

	w = &Wallet{
		SecretKey: sk,
		SpendConditions: consensus.SpendConditions{
			NumSignatures: 1,
			PublicKeys:    []signatures.PublicKey{pk},
		},
		OwnedOutputs: make(map[consensus.OutputID]struct{}),
		SpentOutputs: make(map[consensus.OutputID]struct{}),
	}
	return
}

}
