package consensus

import (
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

// commitDelayedSiacoinOutputDiff applies or reverts a delayedSiacoinOutputDiff.
func (cs *State) commitDelayedSiacoinOutputDiff(dscod modules.DelayedSiacoinOutputDiff, dir modules.DiffDirection) {
	// Sanity check - should not be adding an output twoice, or deleting an
	// output that does not exist.
	if build.DEBUG {
		_, exists := cs.delayedSiacoinOutputs[dscod.MaturityHeight][dscod.ID]
		if exists == (dscod.Direction == dir) {
			panic("rogue delayed siacoin output in commitDelayedSiacoinOutputDiff")
		}
	}

	if dscod.Direction == dir {
		cs.delayedSiacoinOutputs[dscod.MaturityHeight][dscod.ID] = dscod.SiacoinOutput
	} else {
		delete(cs.delayedSiacoinOutputs[dscod.MaturityHeight], dscod.ID)
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

	// Create the filling delayed siacoin output map.
	if dir == modules.DiffApply {
		s.delayedSiacoinOutputs[bn.height+types.MaturityDelay] = make(map[types.SiacoinOutputID]types.SiacoinOutput)
	} else {
		// Skip creating maps for height's that can't have delayed outputs.
		if bn.height > types.MaturityDelay {
			s.delayedSiacoinOutputs[bn.height] = make(map[types.SiacoinOutputID]types.SiacoinOutput)
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
		for _, dscod := range bn.delayedSiacoinOutputDiffs {
			s.commitDelayedSiacoinOutputDiff(dscod, dir)
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
		for i := len(bn.delayedSiacoinOutputDiffs) - 1; i >= 0; i-- {
			s.commitDelayedSiacoinOutputDiff(bn.delayedSiacoinOutputDiffs[i], dir)
		}
	}
	s.commitSiafundPoolDiff(bn.siafundPoolDiff, dir)

	// Delete the emptied siacoin output map.
	if dir == modules.DiffApply {
		// There are no outputs that mature in the first MaturityDelay blocks.
		if bn.height > types.MaturityDelay {
			// Sanity check - the map being deleted should be empty.
			if build.DEBUG {
				if len(s.delayedSiacoinOutputs[bn.height]) != 0 {
					panic("trying to delete a set of delayed outputs that is not empty.")
				}
			}
			delete(s.delayedSiacoinOutputs, bn.height)
		}
	} else {
		// Sanity check - the map being deleted should be empty
		if build.DEBUG {
			if len(s.delayedSiacoinOutputs[bn.height+types.MaturityDelay]) != 0 {
				panic("trying to delete a set of delayed outputs that is not empty.")
			}
		}
		delete(s.delayedSiacoinOutputs, bn.height+types.MaturityDelay)

	}

	// Update the current path.
	if dir == modules.DiffApply {
		s.currentPath = append(s.currentPath, bn.block.ID())
		s.db.AddBlock(bn.block)
	} else {
		s.currentPath = s.currentPath[:len(s.currentPath)-1]
		s.db.RemoveBlock()
	}
}

// generateAndApplyDiff will verify the block and then integrate it into the
// consensus state. These two actions must happen at the same time because
// transactions are allowed to depend on each other. We can't be sure that a
// transaction is valid unless we have applied all of the previous transactions
// in the block, which means we need to apply while we verify.
func (s *State) generateAndApplyDiff(bn *blockNode) error {
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
	s.db.AddBlock(bn.block)
	s.delayedSiacoinOutputs[bn.height+types.MaturityDelay] = make(map[types.SiacoinOutputID]types.SiacoinOutput)

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
		err := s.validTransaction(txn)
		if err != nil {
			s.dosBlocks[bn.block.ID()] = struct{}{}
			s.commitDiffSet(bn, modules.DiffRevert)
			s.deleteNode(bn)
			return err
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

	return nil
}
