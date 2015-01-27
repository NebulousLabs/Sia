package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
)

var (
	NonIsolatedProofErr = errors.New("storage proofs must be alone in transactions")
)

// standard implements the rules outlined in Standard.md, and will return an
// error if any of the rules are violated.
func standard(t consensus.Transaction) (err error) {
	// TODO: Check that the arbitrary data is either prefixed with 'NonSia' or
	// is prefixed with 'HostAnnouncement' plus follows rules for making a host
	// announcement.
	return
}
