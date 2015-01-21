package consensus

import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Sia/crypto"
)

// Exported errors
var (
	ConflictingTransactionErr = errors.New("conflicting transaction exists in transaction pool")
	InvalidSignatureErr       = errors.New("invalid signature in transaction")
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

// applyTransaction() takes a transaction and adds it to the
// ConsensusState, updating the list of contracts, outputs, etc.
func (s *State) applyTransaction(t Transaction) (outputDiffs []OutputDiff, contractDiffs []ContractDiff) {
	// Update the transaction pool to resolve any conflicts.
	s.removeTransactionConflictsFromPool(t)

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
	// Iterate through each input, summing the value, checking for
	// correctness, and creating an InputSignatures object.
	inputSum := Currency(0)
	inputSignaturesMap := make(map[OutputID]*InputSignatures)
	for i, input := range t.Inputs {
		// Check that the input is valid.
		err = s.validInput(input)
		if err != nil {
			return
		}

		// Create the condition for the input signatures and add it to the input signatures map.
		_, exists := inputSignaturesMap[input.OutputID]
		if exists {
			err = errors.New("output spent twice in same transaction")
			return
		}
		inSig := &InputSignatures{
			RemainingSignatures: input.SpendConditions.NumSignatures,
			PossibleKeys:        input.SpendConditions.PublicKeys,
			Index:               i,
		}
		inputSignaturesMap[input.OutputID] = inSig

		// Add the input value to the coin sum.
		inputSum += s.unspentOutputs[input.OutputID].Value
	}

	// Tally up the miner fees and output values.
	outputSum := Currency(0)
	for _, minerFee := range t.MinerFees {
		outputSum += minerFee
	}
	for _, output := range t.Outputs {
		outputSum += output.Value
	}

	// Verify the contracts and tally up the expenditures.
	for _, contract := range t.FileContracts {
		err = s.validContract(contract)
		if err != nil {
			return
		}

		outputSum += contract.Payout
	}

	// Check that all provided proofs are valid.
	for _, proof := range t.StorageProofs {
		err = s.validProof(proof)
		if err != nil {
			return
		}
	}

	// Check that the outputs are less than or equal to the outputs.
	if inputSum != outputSum {
		if DEBUG {
			errorString := fmt.Sprintf("Inputs do not equal outputs for transaction: inputs=%v : outputs=%v", inputSum, outputSum)
			for _, input := range t.Inputs {
				errorString += fmt.Sprintf("\nInput: %v", s.unspentOutputs[input.OutputID].Value)
			}
			for _, fee := range t.MinerFees {
				errorString += fmt.Sprintf("\nMiner Fee: %v", fee)
			}
			for _, output := range t.Outputs {
				errorString += fmt.Sprintf("\nOutput: %v", output.Value)
			}
			for _, fc := range t.FileContracts {
				errorString += fmt.Sprintf("\nContract Fund: %v", fc.Payout)
			}
			err = errors.New(errorString)
			return
		}

		err = errors.New("inputs do not equal outputs for transaction.")
		return
	}

	// Check all of the signatures for validity.
	for i, sig := range t.Signatures {
		// Check that each signature signs a unique pubkey where
		// RemainingSignatures > 0.
		if inputSignaturesMap[sig.InputID].RemainingSignatures == 0 {
			err = errors.New("friviolous signature detected.")
			return
		}
		_, exists := inputSignaturesMap[sig.InputID].UsedKeys[sig.PublicKeyIndex]
		if exists {
			err = errors.New("public key used twice while signing")
			return
		}

		// Check the timelock on the signature.
		if sig.TimeLock > s.height() {
			err = errors.New("signature timelock has not expired")
			return
		}

		// Check that the signature matches the public key.
		sigHash := t.SigHash(i)
		if !crypto.VerifyBytes(sigHash[:], inputSignaturesMap[sig.InputID].PossibleKeys[sig.PublicKeyIndex], sig.Signature) {
			err = InvalidSignatureErr
			return
		}

		// Subtract the number of signatures remaining in the InputSignatures field.
		inputSignaturesMap[sig.InputID].RemainingSignatures -= 1
	}

	// Check that all inputs have been signed by sufficient public keys.
	for _, inputSignatures := range inputSignaturesMap {
		if inputSignatures.RemainingSignatures != 0 {
			if DEBUG {
				err = fmt.Errorf("an input has not been fully signed: %v", inputSignatures.Index)
				return
			}

			err = errors.New("invalid signing of transaction")
			return
		}
	}

	return
}

func (s *State) ValidTransaction(t Transaction) (err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validTransaction(t)
}
