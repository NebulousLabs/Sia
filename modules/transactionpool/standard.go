package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
)

const (
	TransactionSizeLimit = 16 * 1024
)

// checkUnlockConditions looks at the UnlockConditions and verifies that all
// public keys are recognized. Unrecognized public keys are automatically
// accpeted as valid by the state, but should be rejected by miners.
func (tp *TransactionPool) checkUnlockConditions(uc consensus.UnlockConditions) error {
	// Check that all of the public keys are supported algorithms.
	for _, pk := range uc.PublicKeys {
		if pk.Algorithm != consensus.SignatureEntropy &&
			pk.Algorithm != consensus.SignatureEd25519 {
			return errors.New("unrecognized key type in transaction")
		}
	}

	return nil
}

// standard implements the rules outlined in Standard.md, and will return an
// error if any of the rules are violated.
func (tp *TransactionPool) IsStandardTransaction(t consensus.Transaction) (err error) {
	// Check that the size of the transaction does not exceed the standard
	// established in Standard.md
	if len(encoding.Marshal(t)) > TransactionSizeLimit {
		return errors.New("transaction is too large")
	}

	// Check that all public keys are of a recognized type. Need to check all
	// of the UnlockConditions, which currently can appear in 3 separate fields
	// of the transaction.
	for _, sci := range t.SiacoinInputs {
		err = tp.checkUnlockConditions(sci.UnlockConditions)
		if err != nil {
			return
		}
	}
	for _, fct := range t.FileContractTerminations {
		err = tp.checkUnlockConditions(fct.TerminationConditions)
		if err != nil {
			return
		}
	}
	for _, sfi := range t.SiafundInputs {
		err = tp.checkUnlockConditions(sfi.UnlockConditions)
		if err != nil {
			return
		}
	}

	// TODO: Check that the arbitrary data is either prefixed with 'NonSia' or
	// is prefixed with 'HostAnnouncement' plus follows rules for making a host
	// announcement.

	return
}
