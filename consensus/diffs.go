package consensus

// Diffs can be applied or reverted. A bool is used to restrict the value to
// two possibilities.
type DiffDirection bool

const (
	DiffApply  DiffDirection = true
	DiffRevert DiffDirection = false
)

// A SiacoinOutputDiff indicates the addition or removal of a SiacoinOutput in
// the consensus set.
type SiacoinOutputDiff struct {
	Direction     DiffDirection
	ID            SiacoinOutputID
	SiacoinOutput SiacoinOutput
}

// A FileContractDiff indicates the addition or removal of a FileContract in
// the consensus set.
type FileContractDiff struct {
	Direction    DiffDirection
	ID           FileContractID
	FileContract FileContract
}

// A SiafundOutputDiff indicates the addition or removal of a SiafundOutput in
// the consensus set.
type SiafundOutputDiff struct {
	Direction     DiffDirection
	ID            SiafundOutputID
	SiafundOutput SiafundOutput
}

// A SiafundPool diff contains the value of the siafundPool before the block
// was applied, and after the block was applied. When applying the diff, set
// siafundPool to `Adjusted`. When reversing the diff, set siafundPool to
// `Previous`.
type SiafundPoolDiff struct {
	Previous Currency
	Adjusted Currency
}

// commitSiacoinOutputDiff takes a SiacoinOutputDiff and applies it to the
// consensus set.
func (s *State) commitSiacoinOutputDiff(scod SiacoinOutputDiff, dir DiffDirection) {
	if scod.Direction == dir {
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

// commitFileContractDiff takes a FileContractDiff and applies it to the
// consensus set.  `applied` indicates whether the diff is being applied or
// reversed.
func (s *State) commitFileContractDiff(fcd FileContractDiff, dir DiffDirection) {
	if fcd.Direction == dir {
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

// commitSiafundOutputDiff takes a SiafundOutputDiff and applies it to the
// consensus set. `applied` indicates whether the diff is being applied or
// reversed.
func (s *State) commitSiafundOutputDiff(sfod SiafundOutputDiff, dir DiffDirection) {
	if sfod.Direction == dir {
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

// commitSiafundPoolDiff takes a SiafundPoolDiff and applies it to the
// consensus set. `applied` indicates whether the diff is being applied or
// reversed.
func (s *State) commitSiafundPoolDiff(sfpd SiafundPoolDiff, dir DiffDirection) {
	if dir == DiffApply {
		s.siafundPool = sfpd.Adjusted
	} else {
		s.siafundPool = sfpd.Previous
	}
}

// applyDiffSet takes all of the diffs in a block and applies them to the
// consensus set. `applied` indicates whether the diffs are being applied or
// reversed. The ordering is important, because transactions within the same
// block are allowed to depend on each other. When reversing diffs, they must
// be reversed in the opposite order that they were applied.
func (s *State) applyDiffSet(bn *blockNode, dir DiffDirection) {
	// Sanity check
	if DEBUG {
		// Diffs should have already been generated for this node.
		if !bn.diffsGenerated {
			panic("misuse of applyDiffSet - diffs have not been generated!")
		}

		// Current node must be the input node's parent if applied = true, and
		// current node must be the input node if applied = false.
		if dir == DiffApply {
			if bn.parent.block.ID() != s.currentBlockID {
				panic("applying a block node when it's not a valid successor")
			}
		} else {
			if bn.block.ID() != s.currentBlockID {
				panic("applying a block node when it's not a valid successor")
			}
		}
	}

	// The sets of diffs can be applied and reversed in any order, so the
	// ordering is kept when applying and reversing sets. However, within the
	// sets, the diffs must be reversed in the opposite order that they were
	// applied, due to transactions being able to depend on each other. This
	// results is messier for loops when applied is false.
	if dir == DiffApply {
		for _, scod := range bn.siacoinOutputDiffs {
			s.commitSiacoinOutputDiff(scod, dir)
		}
		for _, fcd := range bn.fileContractDiffs {
			s.commitFileContractDiff(fcd, dir)
		}
		for _, sfod := range bn.siafundOutputDiffs {
			s.commitSiafundOutputDiff(sfod, dir)
		}

		s.commitSiafundPoolDiff(bn.siafundPoolDiff, dir)
		s.currentBlockID = bn.block.ID()
		s.currentPath[bn.height] = bn.block.ID()
		s.delayedSiacoinOutputs[bn.height] = bn.delayedSiacoinOutputs
	} else {
		for i := len(bn.siacoinOutputDiffs) - 1; i >= 0; i-- {
			s.commitSiacoinOutputDiff(bn.siacoinOutputDiffs[i], dir)
		}
		for i := len(bn.fileContractDiffs) - 1; i >= 0; i-- {
			s.commitFileContractDiff(bn.fileContractDiffs[i], dir)
		}
		for i := len(bn.siafundOutputDiffs) - 1; i >= 0; i-- {
			s.commitSiafundOutputDiff(bn.siafundOutputDiffs[i], dir)
		}

		s.commitSiafundPoolDiff(bn.siafundPoolDiff, dir)
		s.currentBlockID = bn.parent.block.ID()
		delete(s.currentPath, bn.height)
		delete(s.delayedSiacoinOutputs, bn.height)
	}
}

// generateAndApplyDiff will verify the block and then integrate it into the
// consensus state. These two actions must happen at the same time because
// transactions are allowed to depend on each other. We can't be sure that a
// transaction is valid unless we have applied all of the previous transactions
// in the block, which means we need to apply while we verify.
func (s *State) generateAndApplyDiff(bn *blockNode) (err error) {
	// Sanity check
	if DEBUG {
		// Generate should only be called if the diffs have not yet been
		// generated.
		if bn.diffsGenerated {
			panic("misuse of generateAndApplyDiff")
		}

		// Current node must be the input node's parent.
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
			s.applyDiffSet(bn, DiffRevert)
			return
		}

		s.applyTransaction(bn, txn)
	}

	// After all of the transactions have been applied, 'maintenance' is
	// applied on the block. This includes adding any outputs that have reached
	// maturity, applying any contracts with missed storage proofs, and adding
	// the miner payouts to the list of delayed outputs.
	s.applyMaintenance(bn)

	// The final thing is to update the siafundPoolDiff to indicate where the
	// siafund pool ended up.
	bn.siafundPoolDiff.Adjusted = s.siafundPool

	return
}
