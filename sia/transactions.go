package sia

import (
	"errors"

	"github.com/NebulousLabs/Andromeda/signatures"
)

// Each input has a list of public keys and a required number of signatures.
// InputSignatures keeps track of which public keys have been used and how many
// more signatures are needed.
type InputSignatures struct {
	RemainingSignatures uint64
	PossibleKeys        []signatures.PublicKey
	UsedKeys            map[uint64]struct{}
}

// reverseTransaction removes a given transaction from the
// ConsensusState, making it as though the transaction had never happened.
func (s *State) reverseTransaction(t Transaction) {
	// SCAN THE ARBITRARY DATA AND REMOVE ANY VALID HOSTS FROM THE HOSTDB.

	// Delete all the open contracts created by new contracts.
	for i := range t.FileContracts {
		contractID := t.FileContractID(i)
		delete(s.OpenContracts, contractID)
	}

	// Delete all outputs created by storage proofs.
	for _, sp := range t.StorageProofs {
		openContract := s.OpenContracts[sp.ContractID]
		outputID, err := openContract.FileContract.StorageProofOutputID(openContract.ContractID, s.Height(), true)
		if err != nil {
			panic(err)
		}
		delete(s.UnspentOutputs, outputID)

		// Restore the contract window to being incomplete.
		s.OpenContracts[sp.ContractID].WindowSatisfied = false
	}

	// Delete all financial outputs created by the transaction.
	for i := range t.Outputs {
		delete(s.UnspentOutputs, t.OutputID(i))
	}

	// Restore all inputs to the unspent outputs list.
	for _, input := range t.Inputs {
		s.UnspentOutputs[input.OutputID] = s.SpentOutputs[input.OutputID]
		delete(s.SpentOutputs, input.OutputID)
	}

	// Add the transaction to the transaction pool.
	s.addTransactionToPool(&t)
}

// applyTransaction() takes a transaction and adds it to the
// ConsensusState, updating the list of contracts, outputs, etc.
func (s *State) applyTransaction(t Transaction) {
	// Update the transaction pool to resolve any conflicts.
	s.removeTransactionConflictsFromPool(&t)

	// Remove all inputs from the unspent outputs list.
	for _, input := range t.Inputs {
		s.SpentOutputs[input.OutputID] = s.UnspentOutputs[input.OutputID]
		delete(s.UnspentOutputs, input.OutputID)
	}

	// Add all finanacial outputs to the unspent outputs list.
	for i, output := range t.Outputs {
		s.UnspentOutputs[t.OutputID(i)] = output
	}

	// Add all outputs created by storage proofs.
	for _, sp := range t.StorageProofs {
		s.applyStorageProof(sp)
	}

	// Add all new contracts to the OpenContracts list.
	for i, contract := range t.FileContracts {
		s.addContract(contract, t.FileContractID(i))
	}

	// Scan the arbitrary data for items relevent to the host database.
	s.scanAndApplyHosts(&t)
}

// validInput returns err = nil if the input is valid within the current state,
// otherwise returns an error explaining what wasn't valid.
func (s *State) validInput(input Input) (err error) {
	// Check the input spends an existing and valid output.
	_, exists := s.UnspentOutputs[input.OutputID]
	if !exists {
		err = errors.New("transaction spends a nonexisting output")
		return
	}

	// Check that the spend conditions match the hash listed in the output.
	if input.SpendConditions.CoinAddress() != s.UnspentOutputs[input.OutputID].SpendHash {
		err = errors.New("spend conditions do not match hash")
		return
	}

	// Check the timelock on the spend conditions is expired.
	if input.SpendConditions.TimeLock > s.Height() {
		err = errors.New("output spent before timelock expiry.")
		return
	}

	return
}

// validTransaction returns err = nil if the transaction is valid, otherwise
// returns an error explaining what wasn't valid.
func (s *State) validTransaction(t *Transaction) (err error) {
	// Iterate through each input, summing the value, checking for
	// correctness, and creating an InputSignatures object.
	inputSum := Currency(0)
	inputSignaturesMap := make(map[OutputID]*InputSignatures)
	for _, input := range t.Inputs {
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
		newInputSignatures := &InputSignatures{
			RemainingSignatures: input.SpendConditions.NumSignatures,
			PossibleKeys:        input.SpendConditions.PublicKeys,
		}
		inputSignaturesMap[input.OutputID] = newInputSignatures

		// Add the input value to the coin sum.
		inputSum += s.UnspentOutputs[input.OutputID].Value
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

		outputSum += contract.ContractFund
	}

	// Check that all provided proofs are valid.
	for _, proof := range t.StorageProofs {
		err = s.validProof(proof)
		if err != nil {
			return
		}
	}

	// Check that the outputs are less than or equal to the outputs.
	if inputSum < outputSum {
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
		if sig.TimeLock > s.Height() {
			err = errors.New("signature timelock has not expired")
			return
		}

		// Check that the signature matches the public key.
		sigHash := t.SigHash(i)
		if !signatures.VerifyBytes(sigHash[:], inputSignaturesMap[sig.InputID].PossibleKeys[sig.PublicKeyIndex], sig.Signature) {
			err = errors.New("invalid signature in transaction")
			return
		}

		// Subtract the number of signatures remaining in the InputSignatures field.
		inputSignaturesMap[sig.InputID].RemainingSignatures -= 1
	}

	// Check that all inputs have been signed by sufficient public keys.
	for _, inputSignatures := range inputSignaturesMap {
		if inputSignatures.RemainingSignatures != 0 {
			err = errors.New("an input has not been fully signed")
			return
		}
	}

	return
}

// State.AcceptTransaction() checks for a conflict of the transaction with the
// transaction pool, then checks that the transaction is valid given the
// current state, then adds the transaction to the transaction pool.
// AcceptTransaction() is thread safe, and can be called concurrently.
func (s *State) AcceptTransaction(t Transaction) (err error) {
	s.Lock()
	defer s.Unlock()

	// Check that the transaction is not in conflict with the transaction
	// pool.
	if s.transactionPoolConflict(&t) {
		err = errors.New("conflicting transaction exists in transaction pool")
		return
	}

	// Check that the transaction is potentially valid.
	err = s.validTransaction(&t)
	if err != nil {
		return
	}

	// Add the transaction to the pool.
	s.addTransactionToPool(&t)

	// forward transaction to peers
	s.Server.Broadcast(SendVal('T', t))

	return
}
