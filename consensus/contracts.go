package consensus

import (
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/crypto"
)

func (s *State) storageProofSegment(fcid FileContractID) (index uint64, err error) {
	contract, exists := s.openFileContracts[fcid]
	if !exists {
		err = errors.New("unrecognized file contract id")
		return
	}

	triggerHeight := contract.Start - 1
	triggerBlock, exists := s.blockAtHeight(triggerHeight)
	if !exists {
		err = errors.New("no block found at contract trigger block height")
		return
	}
	triggerID := triggerBlock.ID()

	seed := crypto.HashBytes(append(triggerID[:], fcid[:]...))
	numSegments := int64(crypto.CalculateSegments(contract.FileSize))
	seedInt := new(big.Int).SetBytes(seed[:])
	index = seedInt.Mod(seedInt, big.NewInt(numSegments)).Uint64()
	return
}

// validContract returns err = nil if the contract is valid in the current
// context of the state, and returns an error if something about the contract
// is invalid.
func (s *State) validContract(fc FileContract) error {
	if fc.Start <= s.height() {
		return errors.New("contract must start in the future.")
	}
	if fc.End <= fc.Start {
		return errors.New("contract duration must be at least one block.")
	}
	return nil
}

// validProof returns err = nil if the storage proof provided is valid given
// the state context, otherwise returning an error to indicate what is invalid.
func (s *State) validProof(sp StorageProof) error {
	contract, exists := s.openFileContracts[sp.FileContractID]
	if !exists {
		return errors.New("unrecognized contract id in storage proof")
	}

	// Check that the storage proof itself is valid.
	segmentIndex, err := s.storageProofSegment(sp.FileContractID)
	if err != nil {
		return err
	}
	verified := crypto.VerifySegment(
		sp.Segment,
		sp.HashSet,
		crypto.CalculateSegments(contract.FileSize),
		segmentIndex,
		contract.FileMerkleRoot,
	)
	if !verified {
		return errors.New("provided storage proof is invalid")
	}

	return nil
}

// addContract takes a FileContract and its corresponding ContractID and adds
// it to the state.
func (s *State) applyFileContracts(bn *blockNode, t Transaction) {
	for i, fc := range t.FileContracts {
		// Apply the contract.
		fcid := t.FileContractID(i)
		s.openFileContracts[fcid] = fc

		// Add the diff to the block node.
		fcd := FileContractDiff{
			New:          true,
			ID:           fcid,
			FileContract: fc,
		}
		bn.fileContractDiffs = append(bn.fileContractDiffs, fcd)
	}
	return
}

// splitContractPayout takes a contract payout as input and returns the portion
// of the payout that goes to the pool, as well as the portion that goes to the
// siacoin output. They should add to the original payout.
func splitContractPayout(payout Currency) (poolPortion Currency, outputPortion Currency) {
	poolPortion = payout
	outputPortion = payout
	err := poolPortion.MulFloat(SiafundPortion)
	if err != nil {
		if DEBUG {
			panic("error when doing MulFloat")
		} else {
			return
		}
	}
	err = poolPortion.RoundDown(SiafundCount)
	if err != nil {
		if DEBUG {
			panic("error during RoundDown")
		} else {
			return
		}
	}
	err = outputPortion.Sub(poolPortion)
	if err != nil {
		if DEBUG {
			panic("error during Sub")
		} else {
			return
		}
	}

	// Sanity check - pool portion plus output portion should equal payout.
	if DEBUG {
		tmp := poolPortion
		err = tmp.Add(outputPortion)
		if err != nil {
			panic("err while adding")
		}
		if tmp.Cmp(payout) != 0 {
			panic("siacoins not split correctly during splitContractPayout")
		}
	}

	return
}

// applyStorageProofs takes all of the storage proofs in a transaction and
// applies them to the state, updating the diffs in the block node.
func (s *State) applyStorageProofs(bn *blockNode, t Transaction) {
	for _, sp := range t.StorageProofs {
		// Calculate the new output and its id.
		contract := s.openFileContracts[sp.FileContractID]

		// Get the pool portion and output portion.
		poolPortion, outputPortion := splitContractPayout(contract.Payout)
		sco := SiacoinOutput{
			Value:     outputPortion,
			SpendHash: contract.ValidProofAddress,
		}
		outputID := sp.FileContractID.StorageProofOutputID(true)

		// Sanity check - output should not already exist.
		if DEBUG {
			_, exists := s.unspentSiacoinOutputs[outputID]
			if exists {
				panic("storage proof output already exists")
			}
		}

		// Add the output to the list of delayed outputs, and delete the
		// contract from the state, and add the poolPortion to the siafundPool.
		s.delayedSiacoinOutputs[s.height()][outputID] = sco
		newPool := s.siafundPool
		newPool.Add(poolPortion)
		s.siafundPool = newPool
		delete(s.openFileContracts, sp.FileContractID)

		// update the block node diffs.
		fcd := FileContractDiff{
			New:          false,
			ID:           sp.FileContractID,
			FileContract: contract,
		}
		bn.newDelayedSiacoinOutputs[outputID] = sco
		bn.fileContractDiffs = append(bn.fileContractDiffs, fcd)
	}
	return
}

// applyMissedProof adds outputs to the State to manage a missed storage proof
// on a file contract.
func (s *State) applyMissedProof(bn *blockNode, fc FileContract, fcid FileContractID) {
	poolPortion, outputPortion := splitContractPayout(fc.Payout)

	// Create the output for the missed proof.
	sco := SiacoinOutput{
		Value:     outputPortion,
		SpendHash: fc.MissedProofAddress,
	}
	outputID := fcid.StorageProofOutputID(false)

	// Update the state.
	s.delayedSiacoinOutputs[s.height()][outputID] = sco
	newPool := s.siafundPool
	newPool.Add(poolPortion)
	s.siafundPool = newPool
	delete(s.openFileContracts, fcid)

	// Create the diffs.
	fcd := FileContractDiff{
		New:          false,
		ID:           fcid,
		FileContract: fc,
	}
	bn.fileContractDiffs = append(bn.fileContractDiffs, fcd)
	bn.newDelayedSiacoinOutputs[outputID] = sco
	return
}

func (s *State) applyContractMaintenance(bn *blockNode) {
	// Iterate through all contracts and figure out which ones have expired.
	// Expiring a contract deletes it from the map we are iterating through, so
	// we need to store it and deleted once we're done iterating through the
	// map.
	var expiredContracts []FileContractID
	for id, contract := range s.openFileContracts {
		if s.height() == contract.End {
			expiredContracts = append(expiredContracts, id)
		}
	}

	// Delete all of the contracts that terminated.
	for _, id := range expiredContracts {
		contract := s.openFileContracts[id]
		s.applyMissedProof(bn, contract, id)
	}

	return
}

// StorageProofSegmentIndex takes a contractID and a windowIndex and calculates
// the index of the segment that should be proven on when doing a proof of
// storage.
func (s *State) StorageProofSegment(fcid FileContractID) (index uint64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.storageProofSegment(fcid)
}

func (s *State) ValidContract(fc FileContract) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validContract(fc)
}
