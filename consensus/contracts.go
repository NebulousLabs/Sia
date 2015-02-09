package consensus

// addContract takes a FileContract and its corresponding ContractID and adds
// it to the state.
func (s *State) applyFileContracts(bn *blockNode, t Transaction) {
	for i, fc := range t.FileContracts {
		// TODO: Sanity check.
		// Apply the contract.
		fcid := t.FileContractID(i)
		s.fileContracts[fcid] = fc

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

func (s *State) applyFileContractTerminations(bn *blockNode, t Transaction) {
	for _, fct := range t.FileContractTerminations {
		// Delete the contract.
		fc, exists := s.fileContracts[fct.ParentID]
		// Sanity check - termination should be terminating an existing
		// contract.
		if !exists {
			if DEBUG {
				panic("file contract termination terminates a nonexisting contract")
			} else {
				return
			}
		}
		delete(s.fileContracts, fct.ParentID)

		// Add the diff for the deletion to the block node.
		fcd := FileContractDiff{
			New:          false,
			ID:           fct.ParentID,
			FileContract: fc,
		}
		bn.fileContractDiffs = append(bn.fileContractDiffs, fcd)

		// Add all of the payouts and diffs.
		for i, payout := range fct.Payouts {
			id := fct.ParentID.FileContractTerminationPayoutID(i)
			s.delayedSiacoinOutputs[s.height()][id] = payout
			bn.delayedSiacoinOutputs[id] = payout
		}
	}
}

// splitContractPayout takes a contract payout as input and returns the portion
// of the payout that goes to the pool, as well as the portion that goes to the
// siacoin output. They should add to the original payout.
//
// TODO: move to types.go
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
		// Get the id of the file contract and the siacoin output it creates.
		fileContract := s.fileContracts[sp.ParentID]
		outputID := sp.ParentID.StorageProofOutputID(true)
		// Sanity check - output should not already exist.
		if DEBUG {
			_, exists := s.siacoinOutputs[outputID]
			if exists {
				panic("storage proof output already exists")
			}
		}

		// Get the portion of the payout that goes into the siafundPool and the
		// portion that goes into the siacoin output created for the storage
		// proof.
		poolPortion, outputPortion := splitContractPayout(fileContract.Payout)

		// Create the siacoin output resulting from the storage proof.
		sco := SiacoinOutput{
			Value:      outputPortion,
			UnlockHash: fileContract.ValidProofUnlockHash,
		}

		// Add the output to the list of delayed outputs, delete the
		// contract from the state, and add the poolPortion to the siafundPool.
		s.delayedSiacoinOutputs[s.height()][outputID] = sco
		delete(s.fileContracts, sp.ParentID)
		err := s.siafundPool.Add(poolPortion)
		if DEBUG {
			if err != nil {
				panic(err)
			}
		}

		// update the block node diffs.
		fcd := FileContractDiff{
			New:          false,
			ID:           sp.ParentID,
			FileContract: fileContract,
		}
		bn.delayedSiacoinOutputs[outputID] = sco
		bn.fileContractDiffs = append(bn.fileContractDiffs, fcd)
	}
	return
}
