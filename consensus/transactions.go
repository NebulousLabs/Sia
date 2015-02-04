package consensus

import (
	"errors"
)

// validStorageProofs checks that a transaction follows the limitations placed
// on transactions with storage proofs.
func (t Transaction) validStorageProofs() bool {
	if len(t.StorageProofs) == 0 {
		return true
	}

	if len(t.Outputs) != 0 {
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

// validInput returns err = nil if the input is valid within the current state,
// otherwise returns an error explaining what wasn't valid.
func (s *State) validInput(input Input) (err error) {
	// Check the input spends an existing and valid output.
	_, exists := s.unspentOutputs[input.OutputID]
	if !exists {
		err = errors.New("transaction spends a nonexisting output")
		return
	}

	// Check that the spend conditions match the hash listed in the output.
	if input.SpendConditions.CoinAddress() != s.unspentOutputs[input.OutputID].SpendHash {
		err = errors.New("spend conditions do not match hash")
		return
	}

	// Check the timelock on the spend conditions is expired.
	if input.SpendConditions.TimeLock > s.height() {
		err = errors.New("output spent before timelock expiry.")
		return
	}

	return
}

// ValidTransaction returns err = nil if the transaction is valid, otherwise
// returns an error explaining what wasn't valid.
func (s *State) validTransaction(t Transaction) (err error) {
	// Check that the storage proof guidelines are followed.
	if !t.validStorageProofs() {
		return errors.New("transaction contains storage proofs and conflicts")
	}

	// Validate each input and get the total amount of Currency.
	var inputSum Currency
	for _, input := range t.Inputs {
		// Check that the input is valid.
		err = s.validInput(input)
		if err != nil {
			return
		}

		// Add this input's value
		err = inputSum.Add(s.unspentOutputs[input.OutputID].Value)
		if err != nil {
			return
		}
	}

	// Verify the contracts and tally up the expenditures.
	for _, contract := range t.FileContracts {
		err = s.validContract(contract)
		if err != nil {
			return
		}
	}

	// Check that all provided proofs are valid.
	for _, proof := range t.StorageProofs {
		err = s.validProof(proof)
		if err != nil {
			return
		}
	}

	// Check that the inputs equal the outputs.
	if inputSum.Cmp(t.OutputSum()) != 0 {
		return errors.New("inputs do not equal outputs for transaction.")
	}

	// Check all of the signatures for validity.
	err = s.validSignatures(t)
	if err != nil {
		return
	}

	return
}

// applyTransaction() takes a transaction and adds it to the
// ConsensusState, updating the list of contracts, outputs, etc.
func (s *State) applyTransaction(t Transaction) (outputDiffs []OutputDiff, contractDiffs []ContractDiff) {
	// Remove all inputs from the unspent outputs list.
	for _, input := range t.Inputs {
		// Sanity check - the input must exist within the blockchain, should
		// have already been verified.
		if DEBUG {
			_, exists := s.unspentOutputs[input.OutputID]
			if !exists {
				panic("Applying a transaction with an invalid unspent output!")
			}
		}

		outputDiff := OutputDiff{
			New:    false,
			ID:     input.OutputID,
			Output: s.unspentOutputs[input.OutputID],
		}
		outputDiffs = append(outputDiffs, outputDiff)
		delete(s.unspentOutputs, input.OutputID)
	}

	// Add all finanacial outputs to the unspent outputs list.
	for i, output := range t.Outputs {
		// Sanity check - the output must not exist within the state, should
		// have already been verified.
		if DEBUG {
			_, exists := s.unspentOutputs[t.OutputID(i)]
			if exists {
				panic("applying a  transaction with an invalid new output")
			}
		}

		diff := OutputDiff{
			New:    true,
			ID:     t.OutputID(i),
			Output: output,
		}
		s.unspentOutputs[t.OutputID(i)] = output
		outputDiffs = append(outputDiffs, diff)
	}

	// Add all outputs created by storage proofs.
	for _, sp := range t.StorageProofs {
		outputDiff, contractDiff := s.applyStorageProof(sp)
		outputDiffs = append(outputDiffs, outputDiff)
		contractDiffs = append(contractDiffs, contractDiff)
	}

	// Add all new contracts to the OpenContracts list.
	for i, contract := range t.FileContracts {
		contractDiff := s.applyContract(contract, t.FileContractID(i))
		contractDiffs = append(contractDiffs, contractDiff)
	}
	return
}

// OutputSum returns the sum of all the outputs in the transaction, which must
// match the sum of all the inputs. Outputs created by storage proofs are not
// considered, as they were already considered when the contract was created.
func (t Transaction) OutputSum() (sum Currency) {
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
	for _, output := range t.Outputs {
		sum.Add(output.Value)
	}

	// Check for overflow
	if sum.Overflow() {
		// TODO: ???
	}

	return
}

func (s *State) ValidTransaction(t Transaction) (err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validTransaction(t)
}
