package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
)

// Each input has a list of public keys and a required number of signatures.
// InputSignatures keeps track of which public keys have been used and how many
// more signatures are needed.
type InputSignatures struct {
	RemainingSignatures uint64
	PossibleKeys        []crypto.PublicKey
	UsedKeys            map[uint64]struct{}
	Index               int
}

// OutputSum returns the sum of all the outputs in the transaction, which must
// match the sum of all the inputs. Outputs created by storage proofs are not
// considered, as they were already considered when the contract was created.
func (t Transaction) OutputSum() (sum Currency) {
	// Add the miner fees.
	for _, fee := range t.MinerFees {
		sum += fee
	}

	// Add the contract payouts
	for _, contract := range t.FileContracts {
		sum += contract.Payout
	}

	// Add the outputs
	for _, output := range t.Outputs {
		sum += output.Value
	}

	return
}

func (s *State) ValidSignatures(t Transaction) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validSignatures(t)
}

func (s *State) validSignatures(t Transaction) error {
	// Create the InputSignatures object for each input.
	sigMap := make(map[OutputID]*InputSignatures)
	for i, input := range t.Inputs {
		_, exists := sigMap[input.OutputID]
		if exists {
			return errors.New("output spent twice in the same transaction.")
		}
		inSig := &InputSignatures{
			RemainingSignatures: input.SpendConditions.NumSignatures,
			PossibleKeys:        input.SpendConditions.PublicKeys,
			Index:               i,
		}
		sigMap[input.OutputID] = inSig
	}

	// Check all of the signatures for validity.
	for i, sig := range t.Signatures {
		// Check that each signature signs a unique pubkey where
		// RemainingSignatures > 0.
		if sigMap[sig.InputID].RemainingSignatures == 0 {
			return errors.New("frivolous signature in transaction")
		}
		_, exists := sigMap[sig.InputID].UsedKeys[sig.PublicKeyIndex]
		if exists {
			return errors.New("one public key was used twice while signing an input")
		}
		if sig.TimeLock > s.height() {
			return errors.New("signature used before timelock expiration")
		}

		// Check that the signature matches the public key + data.
		sigHash := t.SigHash(i)
		if !crypto.VerifyBytes(sigHash[:], sigMap[sig.InputID].PossibleKeys[sig.PublicKeyIndex], sig.Signature) {
			return errors.New("signature is invalid")
		}

		// Subtract the number of signatures remaining for this input.
		sigMap[sig.InputID].RemainingSignatures -= 1
	}

	// Check that all inputs have been sufficiently signed.
	for _, reqSigs := range sigMap {
		if reqSigs.RemainingSignatures != 0 {
			return errors.New("some inputs are missing signatures")
		}
	}

	return nil
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
	// Validate each input and get the total amount of Currency.
	inputSum := Currency(0)
	for _, input := range t.Inputs {
		// Check that the input is valid.
		err = s.validInput(input)
		if err != nil {
			return
		}

		// Add the input value to the coin sum.
		inputSum += s.unspentOutputs[input.OutputID].Value
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
	if inputSum != t.OutputSum() {
		err = errors.New("inputs do not equal outputs for transaction.")
		return
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

func (s *State) ValidTransaction(t Transaction) (err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validTransaction(t)
}
