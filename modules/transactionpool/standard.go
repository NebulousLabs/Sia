package transactionpool

import (
	"errors"
	"strings"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

const (
	FileContractConfirmWindow = 3
	TransactionSizeLimit      = 16 * 1024

	PrefixNonSia = "NonSia"
)

var (
	errLargeTransaction = errors.New("transaction is too large")
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
		return errLargeTransaction
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

	// Check that any file contracts do not start for at least
	// FileContractConfirmWindow blocks.
	for _, fc := range t.FileContracts {
		if fc.Start < tp.state.Height()+FileContractConfirmWindow {
			return errors.New("file contract cannot start so close to the current height")
		}
	}

	// Check that any terminations do not become invalid for at least
	// FileContractConfirmWindow blocks.
	for _, fct := range t.FileContractTerminations {
		// Check for the corresponding file contract in the confirmed and
		// unconfirmed sets.
		fc, exists := tp.state.FileContract(fct.ParentID)
		if !exists {
			fc, exists = tp.fileContracts[fct.ParentID]
			if !exists {
				return errors.New("termination submitted for unknown file contract.")
			}
		}
		if fc.Start < tp.stateHeight-FileContractConfirmWindow {
			return errors.New("termination submitted too late")
		}
	}

	// Check that all arbitrary data is prefixed using the recognized set of
	// prefixes.
	for _, data := range t.ArbitraryData {
		if !strings.HasPrefix(data, modules.PrefixHostAnnouncement) &&
			!strings.HasPrefix(data, PrefixNonSia) {
			return errors.New("arbitrary data contains unrecognized prefix")
		}
	}

	return
}
