package miningpool

import (
	"fmt"
	"net"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

type paymentChannel struct {
	// The address to pay to (e.g. the miner's address)
	payTo types.UnlockHash

	// The secret and public key for the address generated when the channel was
	// negotiated. These are needed to update the transaction when the miner
	// submits new work
	sk crypto.SecretKey
	pk crypto.PublicKey

	// The transaction that refunds money to the pool (in case the miner
	// doesn't take their payout
	refundTxn types.Transaction
}

// Communicates with the miner to negotiate a payment channel for sending
// currency from the pool to the miner.
func (mp *MiningPool) rpcNegotiatePaymentChannel(conn net.Conn) error {
	fmt.Println("Negotiating payment channel")
	// Create an address for this specific miner to send money to

	// Negotiate the payment channel
	return nil
}

// sendPayment sends 'amount' through the payment channel whose recipient is
// `addr` by creating a new transaction that sends `amount + oldAmount` to
// `addr` and returning it. This new transaction is meant to then be sent to
// the miner off-chain
func (mp *MiningPool) sendPayment(amount types.Currency, addr types.UnlockHash) (types.Transaction, error) {
	return types.Transaction{}, nil
}
