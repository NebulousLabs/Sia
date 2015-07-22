package miningpool

import (
	"github.com/NebulousLabs/Sia/types"
)

// Communicates with the miner to negotiate a payment channel for sending
// currency from the pool to the miner.
func (mp *MiningPool) CreatePaymentChannel() error {
	return nil
}

// sendCurrency sends `amount` coins to the miner through a payment channel
func (mp *MiningPool) sendCurrency(amount types.Currency, addr types.UnlockHash) error {
	return nil
}
