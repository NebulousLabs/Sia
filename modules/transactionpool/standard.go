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
func standard(t consensus.Transaction) (err error) {
	// Check that the size of the transaction does not exceed the standard
	// established in Standard.md
	encodedTxn := encoding.Marshal(t)
	if len(encodedTxn) > TransactionSizeLimit {
		err = errors.New("transaction is too large")
		return
	}

	// TODO: Check that the arbitrary data is either prefixed with 'NonSia' or
	// is prefixed with 'HostAnnouncement' plus follows rules for making a host
	// announcement.

	return
}
