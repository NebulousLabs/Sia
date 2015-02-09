package consensus

import (
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/crypto"
)

var (
	ErrMissingSiacoinOutput = errors.New("transaction spends a nonexisting siacoin output")
	ErrMissingFileContract  = errors.New("transaction terminates a nonexisting file contract")
	ErrMissingSiafundOutput = errors.New("transaction spends a nonexisting siafund output")
)

// followsStorageProofRule checks that a transaction follows the limitations
// placed on transactions that have storage proofs.
func (t Transaction) FollowsStorageProofRules() error {
	// No storage proofs, no problems.
	if len(t.StorageProofs) == 0 {
		return nil
	}

	// If there are storage proofs, there can be no siacoin outputs, siafund
	// outputs, or new file contracts.
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

// SiacoinOutputSum returns the sum of all the siacoin outputs in the
// transaction, which must match the sum of all the siacoin inputs. Siacoin
// outputs created by storage proofs and siafund outputs are not considered, as
// they were considered when the contract responsible for funding them was
// created.
func (t Transaction) SiacoinOutputSum() (sum Currency, err error) {
	// NOTE: manual overflow checking is performed here to prevent redundant
	// checks.

	// Add the miner fees.
	for _, fee := range t.MinerFees {
		sum.Add(fee)
	}

	// Add the contract payouts
	for _, contract := range t.FileContracts {
		sum.Add(contract.Payout)
	}

	// Add the outputs
	for _, output := range t.SiacoinOutputs {
		sum.Add(output.Value)
	}

	// Check for overflow
	if sum.Overflow() {
		err = ErrOverflow
		return
	}

	return
}

// validUnlockConditions checks that the unlock conditions have been met
// (signatures are checked elsewhere).
func (s *State) validUnlockConditions(uc UnlockConditions, uh UnlockHash) (err error) {
	if uc.UnlockHash() != uh {
		return errors.New("unlock conditions do not match unlock hash")
	}
	if uc.Timelock > s.height() {
		return errors.New("unlock condition timelock has not been met.")
	}

	return
}

// validSiacoinInputs iterates through the inputs of a transaction, summing the
// value of the inputs and checking that the inputs are legal.
func (s *State) validSiacoinInputs(t Transaction) (inputSum Currency, err error) {
	for _, sci := range t.SiacoinInputs {
		// Check that the input spends an existing output, and that the
		// UnlockConditions are legal (signatures checked elsewhere).
		sco, exists := s.siacoinOutputs[sci.ParentID]
		if !exists {
			err = ErrMissingSiacoinOutput
			return
		}
		err = s.validUnlockConditions(sci.UnlockConditions, sco.UnlockHash)
		if err != nil {
			return
		}

		// Add the input value to the sum.
		err = inputSum.Add(sco.Value)
		if err != nil {
			return
		}
	}

	return
}

// validFileContracts iterates through the file contracts of a transaction and
// makes sure that each is legal.
func (s *State) validFileContracts(t Transaction) error {
	for _, fc := range t.FileContracts {
		if fc.Start <= s.height() {
			return errors.New("contract must start in the future.")
		}
		if fc.End <= fc.Start {
			return errors.New("contract duration must be at least one block.")
		}
	}

	return nil
}

// validFileContractTerminations checks that each termination in a transaction
// is legal.
func (s *State) validFileContractTerminations(t Transaction) (err error) {
	for _, fct := range t.FileContractTerminations {
		// Check that the FileContractTermination terminates an existing
		// FileContract.
		fc, exists := s.fileContracts[fct.ParentID]
		if !exists {
			return ErrMissingFileContract
		}
		err = s.validUnlockConditions(fct.TerminationConditions, fc.TerminationHash)
		if err != nil {
			return
		}

		// Check that the payouts in the termination add up to the payout of the
		// contract.
		var payoutSum Currency
		for _, payout := range fct.Payouts {
			err = payoutSum.Add(payout.Value)
			if err != nil {
				return
			}
		}
		if payoutSum.Cmp(fc.Payout) != 0 {
			return errors.New("contract termination has incorrect payouts")
		}
	}

	return
}

// storageProofSegment returns the index of the segment that needs to be proven
// exists in a file contract.
func (s *State) storageProofSegment(fcid FileContractID) (index uint64, err error) {
	// Get the file contract associated with the input id.
	fc, exists := s.fileContracts[fcid]
	if !exists {
		err = errors.New("unrecognized file contract id")
		return
	}

	// Get the id of the trigger block, which is the block at height fc.Start -
	// 1.
	triggerHeight := fc.Start - 1
	triggerBlock, exists := s.blockAtHeight(triggerHeight)
	if !exists {
		err = errors.New("no block found at contract trigger block height")
		return
	}
	triggerID := triggerBlock.ID()

	// Get the index by appending the file contract id to the trigger block and
	// taking the hash, then converting the hash to a numerical value and
	// modding it against the number of segments in the file. The result is a
	// random number in range [0, numSegments]. The probability is very
	// slightly weighted towards the beginning of the file, but because the
	// size difference between the number of segments and the random number
	// being modded, the difference is too small to make any practical
	// difference.
	seed := crypto.HashBytes(append(triggerID[:], fcid[:]...))
	numSegments := int64(crypto.CalculateSegments(fc.FileSize))
	seedInt := new(big.Int).SetBytes(seed[:])
	index = seedInt.Mod(seedInt, big.NewInt(numSegments)).Uint64()
	return
}

// validStorageProofs iterates through the storage proofs of a transaction and
// checks that each is legal.
func (s *State) validStorageProofs(t Transaction) error {
	for _, sp := range t.StorageProofs {
		fc, exists := s.fileContracts[sp.ParentID]
		if !exists {
			return errors.New("unrecognized file contract id in storage proof")
		}

		// Check that the storage proof itself is valid.
		segmentIndex, err := s.storageProofSegment(sp.ParentID)
		if err != nil {
			return err
		}
		verified := crypto.VerifySegment(
			sp.Segment,
			sp.HashSet,
			crypto.CalculateSegments(fc.FileSize),
			segmentIndex,
			fc.FileMerkleRoot,
		)
		if !verified {
			return errors.New("provided storage proof is invalid")
		}
	}

	return nil
}

// validSiacoinInput checks that the given input spends an unspent output, and
// that the UnlockConditions are correct.
func (s *State) validSiafundInput(sfi SiafundInput) (sfo SiafundOutput, err error) {
	sfo, exists := s.siafundOutputs[sfi.ParentID]
	if !exists {
		err = ErrMissingSiafundOutput
		return
	}
	err = s.validUnlockConditions(sfi.UnlockConditions, sfo.UnlockHash)
	if err != nil {
		return
	}

	return
}

// validSiafunds checks that the transaction has valid siafund inputs and
// outputs, and that the sum of the inputs matches the sum of the outputs.
func (s *State) validSiafunds(t Transaction) (err error) {
	// Check that all siafund inputs are valid, and get the total number of
	// input siafunds.
	var siafundInputSum Currency
	for _, sfi := range t.SiafundInputs {
		// Check that the input is valid.
		var sfo SiafundOutput
		sfo, err = s.validSiafundInput(sfi)
		if err != nil {
			return
		}

		// Add this input's value
		err = siafundInputSum.Add(sfo.Value)
		if err != nil {
			return
		}
	}

	// Check that all siafund outputs are valid and that the siafund output sum
	// is equal to the siafund input sum.
	var siafundOutputSum Currency
	for _, sfo := range t.SiafundOutputs {
		// Check that the claimStart is set to 0. Type safety should enforce
		// this, but check anyway.
		if sfo.ClaimStart.Cmp(ZeroCurrency) != 0 {
			return errors.New("invalid siafund output presented")
		}

		// Add this output's value.
		err = siafundOutputSum.Add(sfo.Value)
		if err != nil {
			return
		}
	}
	if siafundOutputSum.Cmp(siafundInputSum) != 0 {
		return errors.New("siafund inputs do not equal siafund outpus within transaction")
	}

	return
}

// validTransaction checks that all fields are valid within the current
// consensus state. If not an error is returned.
func (s *State) validTransaction(t Transaction) (err error) {
	// Check that the storage proof rules are followed.
	err = t.FollowsStorageProofRules()
	if err != nil {
		return
	}

	// Check that all siacoin inputs are valid, and get the total number of
	// input siacoins. Then compare the input siacoins to the output siacoins
	// and return an error if there's a mismatch.
	siacoinInputSum, err := s.validSiacoinInputs(t)
	if err != nil {
		return
	}
	siacoinOutputSum, err := t.SiacoinOutputSum()
	if err != nil {
		return
	}
	if siacoinInputSum.Cmp(siacoinOutputSum) != 0 {
		return errors.New("inputs do not equal outputs for transaction.")
	}

	// Check that all file contracts, terminations, and storage proofs are
	// valid.
	err = s.validFileContracts(t)
	if err != nil {
		return
	}
	err = s.validFileContractTerminations(t)
	if err != nil {
		return
	}
	err = s.validStorageProofs(t)
	if err != nil {
		return
	}

	// Check that the siafund parts of the transaction are valid.
	err = s.validSiafunds(t)
	if err != nil {
		return
	}

	// Check all of the signatures for validity.
	err = s.validSignatures(t)
	if err != nil {
		return
	}

	return
}
