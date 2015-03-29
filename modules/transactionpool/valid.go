package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
)

var (
	ErrBadUnlockConditions      = errors.New("siacoin unlock conditions do not meet required unlock hash")
	ErrSiacoinOverspend         = errors.New("transaction has more siacoin outputs than inputs")
	ErrDoubleSpend              = errors.New("transaction spends an output that was spent by another transaction in the pool")
	ErrUnrecognizedSiacoinInput = errors.New("unrecognized siacoin input in transaction")
)

// validUnconfirmedSiacoins checks that the inputs are all valid in the context
// of the unconfirmed consensus set and that the value of the inputs is equal
// to the value of the outputs. There is an additional check on the unlock
// conditions to see that the hash matches and that the timelock has expired.
func (tp *TransactionPool) validUnconfirmedSiacoins(t consensus.Transaction) (err error) {
	var inputSum consensus.Currency
	for _, sci := range t.SiacoinInputs {
		// Check that the output exists in the unconfirmed set.
		sco, exists := tp.siacoinOutputs[sci.ParentID]
		if !exists {
			return ErrUnrecognizedSiacoinInput
		}

		// Check that the unlock conditions meet the required unlock hash.
		if sci.UnlockConditions.UnlockHash() != sco.UnlockHash {
			return ErrBadUnlockConditions
		}

		inputSum = inputSum.Add(sco.Value)
	}
	if inputSum.Cmp(t.SiacoinOutputSum()) != 0 {
		return ErrSiacoinOverspend
	}
	return
}

// validUnconfirmedStorageProofs checks that a storage proof is valid in the
// context of the confirmed and unconfirmed consensus set.
func (tp *TransactionPool) validUnconfirmedStorageProofs(t consensus.Transaction) (err error) {
	// Check that the file contract is in the unconfirmed set.
	for _, sp := range t.StorageProofs {
		_, exists := tp.state.FileContract(sp.ParentID)
		if !exists {
			return errors.New("storage proof submitted for file contract not in confirmed set.")
		}
	}

	// Check that all of the storage proofs are valid.
	err = tp.state.ValidStorageProofs(t)
	if err != nil {
		return
	}
	return
}

// validUnconfirmedFileContractTerminations checks that all file contract
// terminations are valid in the context of the unconfirmed consensus set.
// There is an additional check for the validity of the unlock conditions and
// the validity of the termination payouts.
func (tp *TransactionPool) validUnconfirmedFileContractTerminations(t consensus.Transaction) (err error) {
	for _, fct := range t.FileContractTerminations {
		// Check for the corresponding file contract in the unconfirmed set.
		fc, exists := tp.fileContracts[fct.ParentID]
		if !exists {
			return errors.New("termination given for unrecognized file contract")
		}

		// Check that the termination conditions match the termination hash.
		if fct.TerminationConditions.UnlockHash() != fc.TerminationHash {
			return errors.New("termination conditions do not meet required termination hash")
		}

		// Check that the termination has been submitted in time.
		if fc.Start < tp.stateHeight {
			return errors.New("termination submitted too late")
		}

		// Check that the payouts in the termination add up to the payout of
		// the contract.
		var payoutSum consensus.Currency
		for _, payout := range fct.Payouts {
			payoutSum = payoutSum.Add(payout.Value)
		}
		if payoutSum.Cmp(fc.Payout) != 0 {
			return errors.New("contract termination has incorrect payouts")
		}
	}
	return
}

// validUnconfirmedSiafunds checks that all siafund inputs are valid in the
// context of the unconfirmed consensus set and that the value of the siafund
// inputs matches the value of the siafund outputs. There is also a check on
// the unlock conditions.
func (tp *TransactionPool) validUnconfirmedSiafunds(t consensus.Transaction) (err error) {
	var inputSum consensus.Currency
	for _, sfi := range t.SiafundInputs {
		// Check that the siafund output being spent exists.
		sfo, exists := tp.siafundOutputs[sfi.ParentID]
		if !exists {
			return errors.New("transaction spends unrecognized siafund output")
		}

		// Check that the unlock conditions match the spend conditions.
		if sfi.UnlockConditions.UnlockHash() != sfo.UnlockHash {
			return errors.New("transaction contains invalid unlock conditions (hash mismatch)")
		}

		// Add this input's value to the inputSum.
		inputSum = inputSum.Add(sfo.Value)
	}

	// Check that the value of the outputs equal the value of the inputs.
	var outputSum consensus.Currency
	for _, sfo := range t.SiafundOutputs {
		outputSum = outputSum.Add(sfo.Value)
	}
	if outputSum.Cmp(inputSum) != 0 {
		return errors.New("siafund inputs do not equal siafund outputs")
	}

	return
}

// validUnconfirmedTransaction checks that the transaction would be valid in a
// block that contained all of the other unconfirmed transactions.
func (tp *TransactionPool) validUnconfirmedTransaction(t consensus.Transaction) (err error) {
	// Check that the transaction follows 'Standard.md' guidelines.
	err = tp.IsStandardTransaction(t)
	if err != nil {
		return
	}

	// Check that the transaction follows general rules - this check looks at
	// rules for transactions contianing storage proofs, the rules for file
	// contracts, and the rules for signatures.
	err = t.StandaloneValid(tp.stateHeight)
	if err != nil {
		return
	}

	// Check the validity of the componenets in the context of the confirmed
	// and unconfirmed set.
	err = tp.validUnconfirmedSiacoins(t)
	if err != nil {
		return
	}
	err = tp.validUnconfirmedStorageProofs(t)
	if err != nil {
		return
	}
	err = tp.validUnconfirmedFileContractTerminations(t)
	if err != nil {
		return
	}
	err = tp.validUnconfirmedSiafunds(t)
	if err != nil {
		return
	}

	return
}
