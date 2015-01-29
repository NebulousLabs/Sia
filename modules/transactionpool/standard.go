package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
)

const (
	TransactionSizeLimit = 64 * 1024
)

// standard implements the rules outlined in Standard.md, and will return an
// error if any of the rules are violated.
func (tp *TransactionPool) IsStandardTransaction(t consensus.Transaction) (err error) {
	// Check that the size of the transaction does not exceed the standard
	// established in Standard.md
	if len(encoding.Marshal(t)) > TransactionSizeLimit {
		err = errors.New("transaction is too large")
		return
	}

	// Check that transactions with storage proofs follow the storage proof
	// rules.
	if len(t.StorageProofs) != 0 {
		if len(t.Inputs) > 1 ||
			len(t.MinerFees) > 1 ||
			len(t.Outputs) > 1 ||
			len(t.FileContracts) != 0 ||
			len(t.ArbitraryData) != 0 {
			err = errors.New("transaction has storage proofs but does not follow the storage proof rules")
			return
		}
	}

	// TODO: Check that the arbitrary data is either prefixed with 'NonSia' or
	// is prefixed with 'HostAnnouncement' plus follows rules for making a host
	// announcement.

	return
}
