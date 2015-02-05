package consensus

import (
	"errors"
)

var (
	MissingOutputErr = errors.New("transaction spends a nonexisting output")
)

// followsStorageProofRule checks that a transaction follows the limitations
// placed on transactions with storage proofs.
func (t Transaction) followsStorageProofRules() bool {
	// No storage proofs, no problems.
	if len(t.StorageProofs) == 0 {
		return true
	}

	// If there are storage proofs, there can be no siacoin outputs, siafund
	// outputs, or new file contracts.
	if len(t.SiacoinOutputs) != 0 {
		return false
	}
	if len(t.FileContracts) != 0 {
		return false
	}
	if len(t.SiafundOutputs) != 0 {
		return false
	}

	return true
}

// validSiacoinInput checks that the given input is valid within the current
// consensus state. If not, an error is returned.
func (s *State) validSiacoinInput(sci SiacoinInput) (sco SiacoinOutput, err error) {
	// Check the input spends an existing and valid output.
	sco, exists := s.unspentSiacoinOutputs[sci.OutputID]
	if !exists {
		err = MissingOutputErr
		return
	}

	// Check that the spend conditions match the hash listed in the output.
	if sci.SpendConditions.CoinAddress() != s.unspentSiacoinOutputs[sci.OutputID].SpendHash {
		err = errors.New("spend conditions do not match hash")
		return
	}

	// Check the timelock on the spend conditions is expired.
	if sci.SpendConditions.TimeLock > s.height() {
		err = errors.New("output spent before timelock expiry.")
		return
	}

	return
}

// validSiafundInput checks that the given input is valid within the current
// consensus state. If not, an error is returned.
func (s *State) validSiafundInput(sfi SiafundInput) (sfo SiafundOutput, err error) {
	// Check the input spends an existing and valid output.
	sfo, exists := s.unspentSiafundOutputs[sfi.OutputID]
	if !exists {
		err = MissingOutputErr
		return
	}

	// Check that the spend conditions match the hash listed in the output.
	if sfi.SpendConditions.CoinAddress() != s.unspentSiafundOutputs[sfi.OutputID].SpendHash {
		err = errors.New("spend conditions do not match hash")
		return
	}

	// Check the timelock on the spend conditions is expired.
	if sfi.SpendConditions.TimeLock > s.height() {
		err = errors.New("output spent before timelock expiry.")
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
		if sfo.claimStart.Cmp(ZeroCurrency) != 0 {
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
	if !t.followsStorageProofRules() {
		return errors.New("transaction contains storage proofs and conflicts")
	}

	// Check that all siacoin inputs are valid, and get the total number of
	// input siacoins.
	var siacoinInputSum Currency
	for _, sci := range t.SiacoinInputs {
		// Check that the input is valid.
		var sco SiacoinOutput
		sco, err = s.validSiacoinInput(sci)
		if err != nil {
			return
		}

		// Add this input's value
		err = siacoinInputSum.Add(sco.Value)
		if err != nil {
			return
		}
	}

	// Check that all contracts and storage proofs are valid.
	for _, contract := range t.FileContracts {
		err = s.validContract(contract)
		if err != nil {
			return
		}
	}
	for _, proof := range t.StorageProofs {
		err = s.validProof(proof)
		if err != nil {
			return
		}
	}

	// Check that the siafund parts of the transaction are valid.
	err = s.validSiafunds(t)
	if err != nil {
		return
	}

	// Calculate the sum of the siacoin outputs and check that it matches the
	// sum of the siacoin inputs.
	siacoinOutputSum, err := t.SiacoinOutputSum()
	if err != nil {
		return
	}
	if siacoinInputSum.Cmp(siacoinOutputSum) != 0 {
		return errors.New("inputs do not equal outputs for transaction.")
	}

	// Check all of the signatures for validity.
	err = s.validSignatures(t)
	if err != nil {
		return
	}

	return
}

// applySiacoinInputs takes all of the siacoin inputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiacoinInputs(bn *blockNode, t Transaction) {
	// Remove all siacoin inputs from the unspent siacoin outputs list.
	for _, sci := range t.SiacoinInputs {
		// Sanity check - the input must exist within the blockchain, should
		// have already been verified.
		if DEBUG {
			_, exists := s.unspentSiacoinOutputs[sci.OutputID]
			if !exists {
				panic("Applying a transaction with an invalid unspent output!")
			}
		}

		scod := SiacoinOutputDiff{
			New:           false,
			ID:            sci.OutputID,
			SiacoinOutput: s.unspentSiacoinOutputs[sci.OutputID],
		}
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
		delete(s.unspentSiacoinOutputs, sci.OutputID)
	}
}

// applySiacoinOutputs takes all of the siacoin outputs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applySiacoinOutputs(bn *blockNode, t Transaction) {
	// Add all siacoin outputs to the unspent siacoin outputs list.
	for i, sco := range t.SiacoinOutputs {
		// Sanity check - the output must not exist within the state, should
		// have already been verified.
		if DEBUG {
			_, exists := s.unspentSiacoinOutputs[t.SiacoinOutputID(i)]
			if exists {
				panic("applying a  transaction with an invalid new output")
			}
		}

		scod := SiacoinOutputDiff{
			New:           true,
			ID:            t.SiacoinOutputID(i),
			SiacoinOutput: sco,
		}
		s.unspentSiacoinOutputs[t.SiacoinOutputID(i)] = sco
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
	}
}

func (s *State) applySiafundInputs(bn *blockNode, t Transaction) {
}

func (s *State) applySiafundOutputs(bn *blockNode, t Transaction) {
}

// applyTransaction takes a transaction and uses the contents to update the
// state of consensus according to the contents of the transaction. The
// transaction is assumed to be valid. A set of diffs are returned that
// represent how the state of consensus has changed. The changes to the
// siafundPool and the delayedSiacoinOutputs are not recorded, as they are
// handled externally.
func (s *State) applyTransaction(bn *blockNode, t Transaction) {
	// Sanity check - the input transaction should be valid.
	if DEBUG {
		err := s.validTransaction(t)
		if err != nil {
			panic("applyTransaction called with an invalid transaction!")
		}
	}

	// Apply each component of the transaction. Miner fees are handled as a
	// separate process.
	s.applySiacoinInputs(bn, t)
	s.applySiacoinOutputs(bn, t)
	s.applyFileContracts(bn, t)
	s.applyStorageProofs(bn, t)
	s.applySiafundInputs(bn, t)
	s.applySiafundOutputs(bn, t)
}

// SiacoinOutputSum returns the sum of all the siacoin outputs in the
// transaction, which must match the sum of all the siacoin inputs. Siacoin
// outputs created by storage proofs and siafund outputs are not considered, as
// they were considered when the contract responsible for funding them was
// created.
//
// TODO: There might be a better place for this.
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

func (s *State) ValidTransaction(t Transaction) (err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validTransaction(t)
}
