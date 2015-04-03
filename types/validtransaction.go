package types

import (
	"errors"

	"github.com/NebulousLabs/Sia/encoding"
)

var (
	ErrMissingSiacoinOutput = errors.New("transaction spends a nonexisting siacoin output")
	ErrMissingFileContract  = errors.New("transaction terminates a nonexisting file contract")
	ErrMissingSiafundOutput = errors.New("transaction spends a nonexisting siafund output")
)

// SiacoinOutputSum returns the sum of all the siacoin outputs in the
// transaction, which must match the sum of all the siacoin inputs. Siacoin
// outputs created by storage proofs and siafund outputs are not considered, as
// they were considered when the contract responsible for funding them was
// created.
func (t Transaction) SiacoinOutputSum() (sum Currency) {
	// Add the miner fees.
	for _, fee := range t.MinerFees {
		sum = sum.Add(fee)
	}

	// Add the contract payouts
	for _, contract := range t.FileContracts {
		sum = sum.Add(contract.Payout)
	}

	// Add the outputs
	for _, output := range t.SiacoinOutputs {
		sum = sum.Add(output.Value)
	}

	return
}

// validUnlockConditions checks that the conditions of uc have been met. The
// height is taken as input so that modules who might be at a different height
// can do the verification without needing to use their own function.
// Additionally, it means that the function does not need to be a method of the
// consensus set.
func validUnlockConditions(uc UnlockConditions, currentHeight BlockHeight) (err error) {
	if uc.Timelock > currentHeight {
		return errors.New("unlock condition timelock has not been met")
	}
	return
}

// correctFileContracts checks that the file contracts adhere to the file
// contract rules.
func (t Transaction) correctFileContracts(currentHeight BlockHeight) error {
	// Check that FileContract rules are being followed.
	for _, fc := range t.FileContracts {
		// Check that start and expiration are reasonable values.
		if fc.Start <= currentHeight {
			return errors.New("contract must start in the future")
		}
		if fc.Expiration <= fc.Start {
			return errors.New("contract duration must be at least one block")
		}

		// Check that the valid proof outputs sum to the payout after the
		// siafund fee has been applied, and check that the missed proof
		// outputs sum to the full payout.
		var validProofOutputSum, missedProofOutputSum Currency
		for _, output := range fc.ValidProofOutputs {
			validProofOutputSum = validProofOutputSum.Add(output.Value)
		}
		for _, output := range fc.MissedProofOutputs {
			missedProofOutputSum = missedProofOutputSum.Add(output.Value)
		}
		outputPortion := fc.Payout.Sub(fc.Tax())
		if validProofOutputSum.Cmp(outputPortion) != 0 {
			return errors.New("contract valid proof outputs do not sum to the payout minus the siafund fee")
		}
		if missedProofOutputSum.Cmp(fc.Payout) != 0 {
			return errors.New("contract missed proof outputs do not sum to the payout")
		}
	}
	return nil
}

// fitsInABlock checks if the transaction is likely to fit in a block.
// Currently there is no limitation on transaction size other than it must fit
// in a block.
func (t Transaction) fitsInABlock() error {
	// Check that the transaction will fit inside of a block, leaving 5kb for
	// overhead.
	if uint64(len(encoding.Marshal(t))) > BlockSizeLimit-5e3 {
		return errors.New("transaction is too large to fit in a block")
	}
	return nil
}

// FollowsStorageProofRules checks that a transaction follows the limitations
// placed on transactions that have storage proofs.
func (t Transaction) followsStorageProofRules() error {
	// No storage proofs, no problems.
	if len(t.StorageProofs) == 0 {
		return nil
	}

	// If there are storage proofs, there can be no siacoin outputs, siafund
	// outputs, new file contracts, or file contract terminations.
	if len(t.SiacoinOutputs) != 0 {
		return errors.New("transaction contains storage proofs and siacoin outputs")
	}
	if len(t.FileContracts) != 0 {
		return errors.New("transaction contains storage proofs and file contracts")
	}
	if len(t.FileContractTerminations) != 0 {
		return errors.New("transaction contains storage proofs and file contract terminations")
	}
	if len(t.SiafundOutputs) != 0 {
		return errors.New("transaction contains storage proofs and siafund outputs")
	}

	return nil
}

// followsMinimumValues checks that all outputs adhere to the rules for the
// minimum allowed value (generally 1).
func (t Transaction) followsMinimumValues() error {
	for _, sco := range t.SiacoinOutputs {
		if sco.Value.IsZero() {
			return errors.New("empty siacoin output not allowed")
		}
	}
	for _, fc := range t.FileContracts {
		if fc.Payout.IsZero() {
			return errors.New("file contract must have non-zero payout")
		}
	}
	for _, sfo := range t.SiafundOutputs {
		// Check that the ClaimStart is set to 0.
		if !sfo.ClaimStart.IsZero() {
			return errors.New("invalid siafund output presented")
		}

		// Outputs must all be at least 1.
		if sfo.Value.IsZero() {
			return errors.New("siafund outputs must have at least 1 siafund")
		}
	}
	return nil
}

// noRepeats checks that a transaction does not spend multiple outputs twice,
// submit two valid storage proofs for the same file contract, etc. We
// frivilously check that a file contract termination and storage proof don't
// act on the same file contract. There is very little overhead for doing so,
// and the check is only frivilous because of the current rule that file
// contract terminations are not valid after the proof window opens.
func (t Transaction) noRepeats() error {
	// Check that there are no repeat instances of siacoin outputs, storage
	// proofs, contract terminations, or siafund outputs.
	siacoinInputs := make(map[SiacoinOutputID]struct{})
	for _, sci := range t.SiacoinInputs {
		_, exists := siacoinInputs[sci.ParentID]
		if exists {
			return errors.New("output spent twice in the same transaction")
		}
		siacoinInputs[sci.ParentID] = struct{}{}
	}
	doneFileContracts := make(map[FileContractID]struct{})
	for _, sp := range t.StorageProofs {
		_, exists := doneFileContracts[sp.ParentID]
		if exists {
			return errors.New("storage proof submitted earlier this transaction")
		}
		doneFileContracts[sp.ParentID] = struct{}{}
	}
	for _, fct := range t.FileContractTerminations {
		_, exists := doneFileContracts[fct.ParentID]
		if exists {
			return errors.New("multiple terminations for the same contract in transaction")
		}
		doneFileContracts[fct.ParentID] = struct{}{}
	}
	siafundInputs := make(map[SiafundOutputID]struct{})
	for _, sfi := range t.SiafundInputs {
		_, exists := siafundInputs[sfi.ParentID]
		if exists {
			return errors.New("siafund output spent twice in the same transaction")
		}
		siafundInputs[sfi.ParentID] = struct{}{}
	}
	return nil
}

// validUnlockConditions checks that all of the unlock conditions in the
// transaction are valid.
func (t Transaction) validUnlockConditions(currentHeight BlockHeight) (err error) {
	for _, sci := range t.SiacoinInputs {
		err = validUnlockConditions(sci.UnlockConditions, currentHeight)
		if err != nil {
			return
		}
	}
	for _, fct := range t.FileContractTerminations {
		err = validUnlockConditions(fct.TerminationConditions, currentHeight)
		if err != nil {
			return
		}
	}
	for _, sfi := range t.SiafundInputs {
		err = validUnlockConditions(sfi.UnlockConditions, currentHeight)
		if err != nil {
			return
		}
	}
	return
}

// StandaloneValid returns an error if a transaction is not valid in any
// context, for example if the same output is spent twice in the same
// transaction. StandaloneValid will not check that all outputs being spent are
// legal outputs, as it has no confirmed or unconfirmed set to look at.
func (t Transaction) StandaloneValid(currentHeight BlockHeight) (err error) {
	err = t.fitsInABlock()
	if err != nil {
		return
	}
	err = t.followsStorageProofRules()
	if err != nil {
		return
	}
	err = t.noRepeats()
	if err != nil {
		return
	}
	err = t.followsMinimumValues()
	if err != nil {
		return
	}
	err = t.correctFileContracts(currentHeight)
	if err != nil {
		return
	}
	err = t.validUnlockConditions(currentHeight)
	if err != nil {
		return
	}
	err = t.validSignatures(currentHeight)
	if err != nil {
		return
	}
	return
}
