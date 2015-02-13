package transactionpool

import (
	"errors"

	"github.com/NebulousLabs/Sia/consensus"
)

// TODO: A lot of the code in these functions is extremely similar to consensus
// code, but I didn't want to touch the consensus code while Luke was going
// through it. But some of this code can be broken out into more pieces and
// that will remove the redundancy.

func (tp *TransactionPool) validUnconfirmedSiacoins(t consensus.Transaction) (err error) {
	var inputSum consensus.Currency
	for _, sci := range t.SiacoinInputs {
		// Check that this output has not already been spent by an unconfirmed
		// transaction.
		_, exists := tp.usedSiacoinOutputs[sci.ParentID]
		if exists {
			return errors.New("transaction contains a double-spend")
		}

		// Check for the output in the confirmed and then unconfirmed output
		// set.
		sco, exists := tp.state.SiacoinOutput(sci.ParentID)
		if !exists {
			sco, exists = tp.siacoinOutputs[sci.ParentID]
			if !exists {
				return errors.New("unrecognized siacoin input in transaction")
			}
		}

		// Check that the unlock conditions are reasonable.
		err = tp.state.ValidUnlockConditions(sci.UnlockConditions, sco.UnlockHash)
		if err != nil {
			return
		}

		inputSum = inputSum.Add(sco.Value)
	}

	// Check that the inputs equal the outputs.
	if inputSum.Cmp(t.SiacoinOutputSum()) != 0 {
		return errors.New("input sum does not equal output sum")
	}

	return
}

func (tp *TransactionPool) validUnconfirmedFileContractTerminations(t consensus.Transaction) (err error) {
	for _, fct := range t.FileContractTerminations {
		// Check that the file contract has not already been terminated.
		_, exists := tp.fileContractTerminations[fct.ParentID]
		if exists {
			return errors.New("termination given for an already terminated file contract")
		}

		// Check for the corresponding FileContract in both the confirmed and
		// unconfirmed set.
		fc, exists := tp.state.FileContract(fct.ParentID)
		if !exists {
			fc, exists = tp.fileContracts[fct.ParentID]
			if !exists {
				return errors.New("termination given for unrecognized file contract")
			}
		}

		// Check that the unlock conditions are resonable.
		err = tp.state.ValidUnlockConditions(fct.TerminationConditions, fc.TerminationHash)
		if err != nil {
			return
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

func (tp *TransactionPool) validUnconfirmedSiafunds(t consensus.Transaction) (err error) {
	var inputSum consensus.Currency
	for _, sfi := range t.SiafundInputs {
		// Check that the siafund output has not already been spent.
		_, exists := tp.usedSiafundOutputs[sfi.ParentID]
		if exists {
			return errors.New("siafund output has already been spent")
		}

		// Check that the siafund output being spent exists.
		sfo, exists := tp.state.SiafundOutput(sfi.ParentID)
		if !exists {
			sfo, exists = tp.siafundOutputs[sfi.ParentID]
			if !exists {
				return errors.New("transaction spends unrecognized siafund output")
			}
		}

		// Check that the unlock conditions are reasonable.
		err = tp.state.ValidUnlockConditions(sfi.UnlockConditions, sfo.UnlockHash)
		if err != nil {
			return
		}

		// Add this input's value to the inputSum.
		inputSum = inputSum.Add(sfo.Value)
	}

	// Go through the outputs, making sure each is valid and summing the number
	// of funds allocated.
	var outputSum consensus.Currency
	for _, sfo := range t.SiafundOutputs {
		if sfo.ClaimStart.Cmp(consensus.ZeroCurrency) != 0 {
			return errors.New("invalid siafund output presented")
		}

		// Add this output's value
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
	err = tp.state.ValidTransactionComponents(t)
	if err != nil {
		return
	}

	// Check the validity of the siacoin inputs and outputs in the context of
	// the unconfirmed transactions.
	err = tp.validUnconfirmedSiacoins(t)
	if err != nil {
		return
	}

	// Check the validity of the file contract terminations in the context of
	// the unconfirmed transactions.
	err = tp.validUnconfirmedFileContractTerminations(t)
	if err != nil {
		return
	}

	// Check the validity of the siafund inputs and outputs in the context of
	// the unconfirmed transactions.
	err = tp.validUnconfirmedSiafunds(t)
	if err != nil {
		return
	}

	return
}
