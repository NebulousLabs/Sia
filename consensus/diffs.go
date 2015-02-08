package consensus

// A block is composed of many transactions. Blocks that have transactions that
// depend on other transactions, but the transactions must all be applied in a
// deterministic order. Transactions cannot have inter-dependencies, meaning
// that an output cannot be created and then spent in the same transaction. As
// far as diffs are concenred, this means that the elements of a transaction
// diff should be able to be applied in any order and the outcome should be the
// same. The elements of a block diff however must be applied in a specific
// order, as transactions may depend on each other.

// An OutputDiff indicates an output that has either been added to or removed
// from the unspent outputs set. New=true means that the output was added when
// the block was applied, and new=false means that the output was deleted when
// the block was applied.
type SiacoinOutputDiff struct {
	New           bool
	ID            SiacoinOutputID
	SiacoinOutput SiacoinOutput
}

type FileContractDiff struct {
	New          bool
	ID           FileContractID
	FileContract FileContract
}

type SiafundOutputDiff struct {
	New           bool
	ID            SiafundOutputID
	SiafundOutput SiafundOutput
}

type SiafundPoolDiff struct {
	Previous Currency
	Adjusted Currency
}

// commitOutputDiff takes an output diff and applies it to the state. Forward
// indicates the direction of the blockchain.
func (s *State) commitSiacoinOutputDiff(scod SiacoinOutputDiff, forward bool) {
	add := scod.New
	if !forward {
		add = !add
	}

	if add {
		// Sanity check - output should not already exist.
		if DEBUG {
			_, exists := s.siacoinOutputs[scod.ID]
			if exists {
				panic("rogue new output in applyOutputDiff")
			}
		}

		s.siacoinOutputs[scod.ID] = scod.SiacoinOutput
	} else {
		// Sanity check - output should exist.
		if DEBUG {
			_, exists := s.siacoinOutputs[scod.ID]
			if !exists {
				panic("rogue non-new output in applyOutputDiff")
			}
		}

		delete(s.siacoinOutputs, scod.ID)
	}
}

// commitContractDiff takes a contract diff and applies it to the state. Forward
// indicates the direction of the blockchain.
func (s *State) commitFileContractDiff(fcd FileContractDiff, forward bool) {
	add := fcd.New
	if !forward {
		add = !add
	}

	if add {
		// Sanity check - contract should not already exist.
		if DEBUG {
			_, exists := s.fileContracts[fcd.ID]
			if exists {
				panic("rogue new contract in applyContractDiff")
			}
		}

		s.fileContracts[fcd.ID] = fcd.FileContract
	} else {
		// Sanity check - contract should exist.
		if DEBUG {
			_, exists := s.fileContracts[fcd.ID]
			if !exists {
				panic("rogue non-new contract in applyContractDiff")
			}
		}

		delete(s.fileContracts, fcd.ID)
	}
}

func (s *State) commitSiafundOutputDiff(sfod SiafundOutputDiff, forward bool) {
	add := sfod.New
	if !forward {
		add = !add
	}

	if add {
		// Sanity check - output should not already exist.
		if DEBUG {
			_, exists := s.siafundOutputs[sfod.ID]
			if exists {
				panic("rogue new output in applyOutputDiff")
			}
		}

		s.siafundOutputs[sfod.ID] = sfod.SiafundOutput
	} else {
		// Sanity check - output should exist.
		if DEBUG {
			_, exists := s.siafundOutputs[sfod.ID]
			if !exists {
				panic("rogue non-new output in applyOutputDiff")
			}
		}

		delete(s.siafundOutputs, sfod.ID)
	}
}

func (s *State) commitSiafundPoolDiff(sfpd SiafundPoolDiff, forward bool) {
	if forward {
		s.siafundPool = sfpd.Adjusted
	} else {
		s.siafundPool = sfpd.Previous
	}
}

func (s *State) applyDiffSet(bn *blockNode, direction bool) {
	// Sanity check - diffs should have already been generated for this node.
	if DEBUG {
		if !bn.diffsGenerated {
			panic("misuse of applyDiffSet - diffs have not been generated!")
		}
	}
	// Sanity check - current node must be the input node's parent if direction
	// = true, and current node must be the input node if direction = false.
	if DEBUG {
		if direction {
			if bn.parent.block.ID() != s.currentBlockID {
				panic("applying a block node when it's not a valid successor")
			}
		} else {
			if bn.block.ID() != s.currentBlockID {
				panic("applying a block node when it's not a valid successor")
			}
		}
	}

	// Apply the siacoin, file contract, and siafund diffs.
	s.commitSiafundPoolDiff(bn.siafundPoolDiff, direction)
	for _, scod := range bn.siacoinOutputDiffs {
		s.commitSiacoinOutputDiff(scod, direction)
	}
	for _, fcd := range bn.fileContractDiffs {
		s.commitFileContractDiff(fcd, direction)
	}
	for _, sfod := range bn.siafundOutputDiffs {
		s.commitSiafundOutputDiff(sfod, direction)
	}

	// Manage the delayed outputs that have been created by the node.
	if direction {
		s.delayedSiacoinOutputs[bn.height] = bn.delayedSiacoinOutputs
	} else {
		delete(s.delayedSiacoinOutputs, bn.height)
	}

	// Update the current path and currentBlockID
	if direction {
		s.currentBlockID = bn.block.ID()
		s.currentPath[bn.height] = bn.block.ID()
	} else {
		delete(s.currentPath, bn.height)
		s.currentBlockID = bn.parent.block.ID()
	}
}

// applyMinerSubsidy adds all of the outputs recorded in the MinerPayouts to
// the state, and returns the corresponding set of diffs.
func (s *State) applyMinerSubsidy(bn *blockNode) {
	for i, payout := range bn.block.MinerPayouts {
		id := bn.block.MinerPayoutID(i)
		s.delayedSiacoinOutputs[s.height()][id] = payout
		bn.delayedSiacoinOutputs[id] = payout
	}
	return
}

// applyDelayedSiacoinOutputMaintenance goes through all of the outputs that
// have matured and adds them to the list of siacoinOutputs.
func (s *State) applyDelayedSiacoinOutputMaintenance(bn *blockNode) {
	for id, sco := range s.delayedSiacoinOutputs[bn.height-MaturityDelay] {
		// Sanity check - the output should not already be in the
		// siacoinOuptuts list.
		if DEBUG {
			_, exists := s.siacoinOutputs[id]
			if exists {
				panic("trying to add a delayed output when the output is already there")
			}
		}
		s.siacoinOutputs[id] = sco

		// Create and add the diff.
		scod := SiacoinOutputDiff{
			New:           true,
			ID:            id,
			SiacoinOutput: sco,
		}
		bn.siacoinOutputDiffs = append(bn.siacoinOutputDiffs, scod)
	}
}

// generateAndApplyDiff will verify the block and then integrate it into the
// consensus state.
func (s *State) generateAndApplyDiff(bn *blockNode) (err error) {
	// Sanity check - generate should only be called if the diffs have not yet
	// been generated - current node must be the input node's parent.
	if DEBUG {
		if bn.diffsGenerated {
			panic("misuse of generateAndApplyDiff")
		}
		if bn.parent.block.ID() != s.currentBlockID {
			panic("applying a block node when it's not a valid successor")
		}
	}

	// Update the state to point to the new block.
	s.currentBlockID = bn.block.ID()
	s.currentPath[bn.height] = bn.block.ID()
	s.delayedSiacoinOutputs[s.height()] = make(map[SiacoinOutputID]SiacoinOutput)

	// diffsGenerated is set to true as soon as we start changing the set of
	// diffs in the block node. If at any point the block is found to be
	// invalid, the diffs can be safely reversed from whatever point.
	bn.diffsGenerated = true

	// The first diff to be applied is to mark what the starting siafundPool balance
	// is.
	bn.siafundPoolDiff.Previous = s.siafundPool

	// Validate and apply each transaction in the block. They cannot be
	// validated all at once because some transactions may not be valid until
	// previous transactions have been applied.
	for _, txn := range bn.block.Transactions {
		err = s.validTransaction(txn)
		if err != nil {
			direction := false // set direction to false because the diffs are being removed.
			s.applyDiffSet(bn, direction)
			return
		}

		s.applyTransaction(bn, txn)
	}

	// Perform maintanence on all open contracts.
	s.applyContractMaintenance(bn)
	s.applyDelayedSiacoinOutputMaintenance(bn)
	s.applyMinerSubsidy(bn)

	// The final diff to be applied is to mark what the ending siafundPool
	// balance is.
	bn.siafundPoolDiff.Adjusted = s.siafundPool

	return
}
