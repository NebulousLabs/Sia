package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// diffs.go contains all of the functions related to diffs in the consensus
// set. Each block changes the consensus set in a deterministic way, these
// changes are recorded as diffs for easy rewinding and reapplying. The diffs
// are created, applied, reverted, and queried in this file.

// commitSiacoinOutputDiff applies or reverts a SiacoinOutputDiff.
func (s *State) commitSiacoinOutputDiff(scod modules.SiacoinOutputDiff, dir modules.DiffDirection) {
	// Sanity check - should not be adding an output twice, or deleting an
	// output that does not exist.
	if build.DEBUG {
		_, exists := s.siacoinOutputs[scod.ID]
		if exists == (scod.Direction == dir) {
			panic("rogue siacoin output in commitSiacoinOutputDiff")
		}
	}

	if scod.Direction == dir {
		s.siacoinOutputs[scod.ID] = scod.SiacoinOutput
	} else {
		delete(s.siacoinOutputs, scod.ID)
	}
}

// commitFileContractDiff applies or reverts a FileContractDiff.
func (s *State) commitFileContractDiff(fcd modules.FileContractDiff, dir modules.DiffDirection) {
	// Sanity check - should not be adding a contract twice, or deleting a
	// contract that does not exist.
	if build.DEBUG {
		_, exists := s.fileContracts[fcd.ID]
		if exists == (fcd.Direction == dir) {
			panic("rogue file contract in commitFileContractDiff")
		}
	}

	if fcd.Direction == dir {
		s.fileContracts[fcd.ID] = fcd.FileContract
	} else {
		delete(s.fileContracts, fcd.ID)
	}
}

// commitSiafundOutputDiff applies or reverts a SiafundOutputDiff.
func (s *State) commitSiafundOutputDiff(sfod modules.SiafundOutputDiff, dir modules.DiffDirection) {
	// Sanity check - should not be adding an output twice, or deleting an
	// output that does not exist.
	if build.DEBUG {
		_, exists := s.siafundOutputs[sfod.ID]
		if exists == (sfod.Direction == dir) {
			panic("rogue siafund output in commitSiafundOutputDiff")
		}
	}

	if sfod.Direction == dir {
		s.siafundOutputs[sfod.ID] = sfod.SiafundOutput
	} else {
		delete(s.siafundOutputs, sfod.ID)
	}
}

// commitSiafundPoolDiff applies or reverts a SiafundPoolDiff.
func (s *State) commitSiafundPoolDiff(sfpd modules.SiafundPoolDiff, dir modules.DiffDirection) {
	if dir == modules.DiffApply {
		s.siafundPool = sfpd.Adjusted
	} else {
		s.siafundPool = sfpd.Previous
	}
}

// commitDiffSet applies or reverts the diffs in a blockNode.
func (s *State) commitDiffSet(bn *blockNode, dir modules.DiffDirection) {
	// Sanity check
	if build.DEBUG {
		// Diffs should have already been generated for this node.
		if !bn.diffsGenerated {
			panic("misuse of applyDiffSet - diffs have not been generated!")
		}

		// Current node must be the input node's parent if applying, and
		// current node must be the input node if reverting.
		if dir == modules.DiffApply {
			if bn.parent.block.ID() != s.currentBlockID() {
				panic("applying a block node when it's not a valid successor")
			}
		} else {
			if bn.block.ID() != s.currentBlockID() {
				panic("applying a block node when it's not a valid successor")
			}
		}
	}

	// Apply each of the diffs.
	if dir == modules.DiffApply {
		for _, scod := range bn.siacoinOutputDiffs {
			s.commitSiacoinOutputDiff(scod, dir)
		}
		for _, fcd := range bn.fileContractDiffs {
			s.commitFileContractDiff(fcd, dir)
		}
		for _, sfod := range bn.siafundOutputDiffs {
			s.commitSiafundOutputDiff(sfod, dir)
		}
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
	}
	s.commitSiafundPoolDiff(bn.siafundPoolDiff, dir)

	// Update the State's metadata
	if dir == modules.DiffApply {
		s.currentPath = append(s.currentPath, bn.block.ID())
		s.delayedSiacoinOutputs[bn.height] = bn.delayedSiacoinOutputs
	} else {
		s.currentPath = s.currentPath[:len(s.currentPath)-1]
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
	if build.DEBUG {
		// Generate should only be called if the diffs have not yet been
		// generated.
		if bn.diffsGenerated {
			panic("misuse of generateAndApplyDiff")
		}

		// Current node must be the input node's parent.
		if bn.parent.block.ID() != s.currentBlockID() {
			panic("applying a block node when it's not a valid successor")
		}
	}

	// Update the state to point to the new block.
	s.currentPath = append(s.currentPath, bn.block.ID())
	s.delayedSiacoinOutputs[s.height()] = make(map[types.SiacoinOutputID]types.SiacoinOutput)

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
			s.badBlocks[bn.block.ID()] = struct{}{}
			s.deleteNode(bn)
			s.commitDiffSet(bn, modules.DiffRevert)
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

// BlockDiffs returns the diffs created by the input block.
func (s *State) BlockDiffs(bid types.BlockID) (scods []modules.SiacoinOutputDiff, fcds []modules.FileContractDiff, sfods []modules.SiafundOutputDiff, sfpd modules.SiafundPoolDiff, err error) {
	id := s.mu.RLock()
	defer s.mu.RUnlock(id)

	bn, exists := s.blockMap[bid]
	if !exists {
		err = errors.New("could not find block")
		return
	}

	scods = bn.siacoinOutputDiffs
	fcds = bn.fileContractDiffs
	sfods = bn.siafundOutputDiffs
	sfpd = bn.siafundPoolDiff
	return
}
