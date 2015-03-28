package consensus

import (
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/crypto"
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
	if len(encoding.Marshal(t)) > BlockSizeLimit-5e3 {
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
		if sco.Value.Cmp(ZeroCurrency) < 1 {
			return errors.New("empty siacoin output not allowed")
		}
	}
	for _, fc := range t.FileContracts {
		if fc.Payout.Cmp(ZeroCurrency) < 1 {
			return errors.New("file contract must have non-zero payout")
		}
	}
	for _, sfo := range t.SiafundOutputs {
		// Check that the claimStart is set to 0.
		if sfo.ClaimStart.Cmp(ZeroCurrency) != 0 {
			return errors.New("invalid siafund output presented")
		}

		// Outputs must all be at least 1.
		if sfo.Value.Cmp(ZeroCurrency) < 1 {
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

// validSiacoins checks that the siacoin inputs and outputs are valid in the
// context of the current consensus set.
func (s *State) validSiacoins(t Transaction) (err error) {
	var inputSum Currency
	for _, sci := range t.SiacoinInputs {
		// Check that the input spends an existing output.
		sco, exists := s.siacoinOutputs[sci.ParentID]
		if !exists {
			return ErrMissingSiacoinOutput
		}

		// Check that the unlock conditions match the required unlock hash.
		if sci.UnlockConditions.UnlockHash() != sco.UnlockHash {
			return errors.New("siacoin unlock conditions do not meet required unlock hash")
		}

		inputSum = inputSum.Add(sco.Value)
	}
	if inputSum.Cmp(t.SiacoinOutputSum()) != 0 {
		return errors.New("inputs do not equal outputs for transaction")
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

	// Get the ID of the trigger block.
	triggerHeight := fc.Start - 1
	if triggerHeight > s.height() {
		err = errors.New("no block found at contract trigger block height")
		return
	}
	triggerID := s.currentPath[triggerHeight]

	// Get the index by appending the file contract ID to the trigger block and
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

// validStorageProofs checks that the storage proofs are valid in the context
// of the consensus set.
func (s *State) validStorageProofs(t Transaction) error {
	for _, sp := range t.StorageProofs {
		fc, exists := s.fileContracts[sp.ParentID]
		if !exists {
			return errors.New("unrecognized file contract ID in storage proof")
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

// validFileContractTerminations checks that each file contract termination is
// valid in the context of the current consensus set.
func (s *State) validFileContractTerminations(t Transaction) (err error) {
	for _, fct := range t.FileContractTerminations {
		// Check that the FileContractTermination terminates an existing
		// FileContract.
		fc, exists := s.fileContracts[fct.ParentID]
		if !exists {
			return ErrMissingFileContract
		}

		// Check that the height is less than fc.Start - terminations are not
		// allowed to be submitted once the storage proof window has opened.
		// This reduces complexity for unconfirmed transactions.
		if fc.Start < s.height() {
			return errors.New("contract termination submitted too late")
		}

		// Check that the unlock conditions match the unlock hash.
		if fct.TerminationConditions.UnlockHash() != fc.TerminationHash {
			return errors.New("termination conditions don't match required termination hash")
		}

		// Check that the payouts in the termination add up to the payout of the
		// contract.
		var payoutSum Currency
		for _, payout := range fct.Payouts {
			payoutSum = payoutSum.Add(payout.Value)
		}
		if payoutSum.Cmp(fc.Payout) != 0 {
			return errors.New("contract termination has incorrect payouts")
		}
	}

	return
}

// validSiafunds checks that the siafund portions of the transaction are valid
// in the context of the consensus set.
func (s *State) validSiafunds(t Transaction) (err error) {
	// Compare the number of input siafunds to the output siafunds.
	var siafundInputSum Currency
	var siafundOutputSum Currency
	for _, sfi := range t.SiafundInputs {
		sfo, exists := s.siafundOutputs[sfi.ParentID]
		if !exists {
			return ErrMissingSiafundOutput
		}

		// Check the unlock conditions match the unlock hash.
		if sfi.UnlockConditions.UnlockHash() != sfo.UnlockHash {
			return errors.New("unlock conditions don't match required unlock hash")
		}

		siafundInputSum = siafundInputSum.Add(sfo.Value)
	}
	for _, sfo := range t.SiafundOutputs {
		siafundOutputSum = siafundOutputSum.Add(sfo.Value)
	}
	if siafundOutputSum.Cmp(siafundInputSum) != 0 {
		return errors.New("siafund inputs do not equal siafund outpus within transaction")
	}
	return
}

// validTransaction checks that all fields are valid within the current
// consensus state. If not an error is returned.
func (s *State) validTransaction(t Transaction) (err error) {
	// StandaloneValid will check things like signatures and properties that
	// should be inherent to the transaction. (storage proof rules, etc.)
	err = t.StandaloneValid(s.height())
	if err != nil {
		return
	}

	// Check that each portion of the transaction is legal given the current
	// consensus set.
	err = s.validSiacoins(t)
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
	err = s.validSiafunds(t)
	if err != nil {
		return
	}

	return
}

// ValidStorageProofs checks that the storage proofs are valid in the context
// of the consensus set.
func (s *State) ValidStorageProofs(t Transaction) (err error) {
	id := s.mu.RLock()
	defer s.mu.RUnlock(id)
	return s.validStorageProofs(t)
}
