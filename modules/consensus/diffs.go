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

var (
	errApplySiafundPoolDiffMismatch      = errors.New("committing a siafund pool diff with an invalid 'previous' field")
	errBadCommitSiacoinOutputDiff        = errors.New("rogue siacoin output diff in commitSiacoinOutputDiff")
	errBadCommitFileContractDiff         = errors.New("rogue file contract diff in commitFileContractDiff")
	errBadCommitSiafundOutputDiff        = errors.New("rogue siafund output diff in commitSiafundOutputDiff")
	errBadCommitDelayedSiacoinOutputDiff = errors.New("rogue delayed siacoin output diff in commitSiacoinOutputDiff")
	errBadExpirationPointer              = errors.New("deleting a file contract that has a file pointer to a nonexistant map")
	errBadMaturityHeight                 = errors.New("delayed siacoin output diff was submitted with illegal maturity height")
	errCreatingExistingUpcomingMap       = errors.New("creating an existing upcoming map")
	errDeletingNonEmptyDelayedMap        = errors.New("deleting a delayed siacoin output map that is not empty")
	errDiffsNotGenerated                 = errors.New("applying diff set before generating errors")
	errExistingFileContractExpiration    = errors.New("creating a pointer to a file contract expiration that already exists")
	errInvalidSuccessor                  = errors.New("generating diffs for a block that's an invalid successsor to the current block")
	errNegativePoolAdjustment            = errors.New("committing a siafund pool diff with a negative adjustment")
	errNonApplySiafundPoolDiff           = errors.New("commiting a siafund pool diff that doesn't have the 'apply' direction")
	errRegenerateDiffs                   = errors.New("cannot call generateAndApplyDiffs on a node for which diffs were already generated")
	errRevertSiafundPoolDiffMismatch     = errors.New("committing a siafund pool diff with an invalid 'adjusted' field")
	errWrongAppliedDiffSet               = errors.New("applying a diff set that isn't the current block")
	errWrongRevertDiffSet                = errors.New("reverting a diff set that isn't the current block")
)

// commitSiacoinOutputDiff applies or reverts a SiacoinOutputDiff.
func (cs *ConsensusSet) commitSiacoinOutputDiff(scod modules.SiacoinOutputDiff, dir modules.DiffDirection) {
	// Sanity check - should not be adding an output twice, or deleting an
	// output that does not exist.
	if build.DEBUG {
		_, exists := cs.siacoinOutputs[scod.ID]
		if exists == (scod.Direction == dir) {
			panic(errBadCommitSiacoinOutputDiff)
		}
	}

	if scod.Direction == dir {
		cs.siacoinOutputs[scod.ID] = scod.SiacoinOutput
	} else {
		delete(cs.siacoinOutputs, scod.ID)
	}
}

// commitFileContractDiff applies or reverts a FileContractDiff.
func (cs *ConsensusSet) commitFileContractDiff(fcd modules.FileContractDiff, dir modules.DiffDirection) {
	// Sanity check - should not be adding a contract twice, or deleting a
	// contract that does not exist.
	if build.DEBUG {
		_, exists := cs.fileContracts[fcd.ID]
		if exists == (fcd.Direction == dir) {
			panic(errBadCommitFileContractDiff)
		}
	}

	if fcd.Direction == dir {
		cs.fileContracts[fcd.ID] = fcd.FileContract

		// Put a file contract into the file contract expirations map.
		_, exists := cs.fileContractExpirations[fcd.FileContract.WindowEnd]
		if !exists {
			cs.fileContractExpirations[fcd.FileContract.WindowEnd] = make(map[types.FileContractID]struct{})
		}

		// Sanity check - file contract expiration pointer should not already
		// exist.
		if build.DEBUG {
			_, exists := cs.fileContractExpirations[fcd.FileContract.WindowEnd][fcd.ID]
			if exists {
				panic(errExistingFileContractExpiration)
			}
		}
		cs.fileContractExpirations[fcd.FileContract.WindowEnd][fcd.ID] = struct{}{}
	} else {
		delete(cs.fileContracts, fcd.ID)

		if build.DEBUG {
			_, exists := cs.fileContractExpirations[fcd.FileContract.WindowEnd]
			if !exists {
				panic(errBadExpirationPointer)
			}
			_, exists = cs.fileContractExpirations[fcd.FileContract.WindowEnd][fcd.ID]
			if !exists {
				panic(errBadExpirationPointer)
			}
		}
		delete(cs.fileContractExpirations[fcd.FileContract.WindowEnd], fcd.ID)
	}
}

// commitSiafundOutputDiff applies or reverts a SiafundOutputDiff.
func (cs *ConsensusSet) commitSiafundOutputDiff(sfod modules.SiafundOutputDiff, dir modules.DiffDirection) {
	// Sanity check - should not be adding an output twice, or deleting an
	// output that does not exist.
	if build.DEBUG {
		_, exists := cs.siafundOutputs[sfod.ID]
		if exists == (sfod.Direction == dir) {
			panic(errBadCommitSiafundOutputDiff)
		}
	}

	if sfod.Direction == dir {
		cs.siafundOutputs[sfod.ID] = sfod.SiafundOutput
	} else {
		delete(cs.siafundOutputs, sfod.ID)
	}
}

// commitDelayedSiacoinOutputDiff applies or reverts a delayedSiacoinOutputDiff.
func (cs *ConsensusSet) commitDelayedSiacoinOutputDiff(dscod modules.DelayedSiacoinOutputDiff, dir modules.DiffDirection) {
	// Sanity check - should not be adding an output twice, or deleting an
	// output that does not exist.
	if build.DEBUG {
		_, exists := cs.delayedSiacoinOutputs[dscod.MaturityHeight]
		if !exists {
			panic(errBadMaturityHeight)
		}
		_, exists = cs.delayedSiacoinOutputs[dscod.MaturityHeight][dscod.ID]
		if exists == (dscod.Direction == dir) {
			panic(errBadCommitDelayedSiacoinOutputDiff)
		}
	}

	if dscod.Direction == dir {
		cs.delayedSiacoinOutputs[dscod.MaturityHeight][dscod.ID] = dscod.SiacoinOutput
	} else {
		delete(cs.delayedSiacoinOutputs[dscod.MaturityHeight], dscod.ID)
	}
}

// commitSiafundPoolDiff applies or reverts a SiafundPoolDiff.
func (cs *ConsensusSet) commitSiafundPoolDiff(sfpd modules.SiafundPoolDiff, dir modules.DiffDirection) {
	// Sanity check - siafund pool should only ever increase.
	if build.DEBUG {
		if sfpd.Adjusted.Cmp(sfpd.Previous) < 0 {
			panic(errNegativePoolAdjustment)
		}
		if sfpd.Direction != modules.DiffApply {
			panic(errNonApplySiafundPoolDiff)
		}
	}

	if dir == modules.DiffApply {
		// Sanity check - sfpd.Previous should equal the current siafund pool.
		if build.DEBUG {
			if cs.siafundPool.Cmp(sfpd.Previous) != 0 {
				panic(errApplySiafundPoolDiffMismatch)
			}
		}
		cs.siafundPool = sfpd.Adjusted
	} else {
		// Sanity check - sfpd.Adjusted should equal the current siafund pool.
		if build.DEBUG {
			if cs.siafundPool.Cmp(sfpd.Adjusted) != 0 {
				panic(errRevertSiafundPoolDiffMismatch)
			}
		}
		cs.siafundPool = sfpd.Previous
	}
}

// commitDiffSetSanity performs a series of sanity checks before commiting a
// diff set.
func (cs *ConsensusSet) commitDiffSetSanity(bn *blockNode, dir modules.DiffDirection) {
	// Sanity checks.
	if build.DEBUG {
		// Diffs should have already been generated for this node.
		if !bn.diffsGenerated {
			panic(errDiffsNotGenerated)
		}

		// Current node must be the input node's parent if applying, and
		// current node must be the input node if reverting.
		if dir == modules.DiffApply {
			if bn.parent.block.ID() != cs.currentBlockID() {
				panic(errWrongAppliedDiffSet)
			}
		} else {
			if bn.block.ID() != cs.currentBlockID() {
				panic(errWrongRevertDiffSet)
			}
		}
	}
}

// createUpcomingDelayeOutputdMaps creates the delayed siacoin output maps that
// will be used when applying delayed siacoin outputs in the diff set.
func (cs *ConsensusSet) createUpcomingDelayedOutputMaps(bn *blockNode, dir modules.DiffDirection) {
	if dir == modules.DiffApply {
		if build.DEBUG {
			// Sanity check - the output map being created should not already
			// exist.
			_, exists := cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay]
			if exists {
				panic(errCreatingExistingUpcomingMap)
			}
		}
		cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay] = make(map[types.SiacoinOutputID]types.SiacoinOutput)
	} else {
		// Skip creating maps for heights that can't have delayed outputs.
		if bn.height > types.MaturityDelay {
			// Sanity check - the output map being created should not already
			// exist.
			if build.DEBUG {
				_, exists := cs.delayedSiacoinOutputs[bn.height]
				if exists {
					panic(errCreatingExistingUpcomingMap)
				}
			}
			cs.delayedSiacoinOutputs[bn.height] = make(map[types.SiacoinOutputID]types.SiacoinOutput)
		}
	}
}

// commitNodeDiffs commits all of the diffs in a block node.
func (cs *ConsensusSet) commitNodeDiffs(bn *blockNode, dir modules.DiffDirection) {
	if dir == modules.DiffApply {
		for _, scod := range bn.siacoinOutputDiffs {
			cs.commitSiacoinOutputDiff(scod, dir)
		}
		for _, fcd := range bn.fileContractDiffs {
			cs.commitFileContractDiff(fcd, dir)
		}
		for _, sfod := range bn.siafundOutputDiffs {
			cs.commitSiafundOutputDiff(sfod, dir)
		}
		for _, dscod := range bn.delayedSiacoinOutputDiffs {
			cs.commitDelayedSiacoinOutputDiff(dscod, dir)
		}
		for _, sfpd := range bn.siafundPoolDiffs {
			cs.commitSiafundPoolDiff(sfpd, dir)
		}
	} else {
		for i := len(bn.siacoinOutputDiffs) - 1; i >= 0; i-- {
			cs.commitSiacoinOutputDiff(bn.siacoinOutputDiffs[i], dir)
		}
		for i := len(bn.fileContractDiffs) - 1; i >= 0; i-- {
			cs.commitFileContractDiff(bn.fileContractDiffs[i], dir)
		}
		for i := len(bn.siafundOutputDiffs) - 1; i >= 0; i-- {
			cs.commitSiafundOutputDiff(bn.siafundOutputDiffs[i], dir)
		}
		for i := len(bn.delayedSiacoinOutputDiffs) - 1; i >= 0; i-- {
			cs.commitDelayedSiacoinOutputDiff(bn.delayedSiacoinOutputDiffs[i], dir)
		}
		for i := len(bn.siafundPoolDiffs) - 1; i >= 0; i-- {
			cs.commitSiafundPoolDiff(bn.siafundPoolDiffs[i], dir)
		}
	}
}

// deleteObsoleteDelayedOutputMaps deletes the delayed siacoin output maps that
// are no longer in use.
func (cs *ConsensusSet) deleteObsoleteDelayedOutputMaps(bn *blockNode, dir modules.DiffDirection) {
	if dir == modules.DiffApply {
		// There are no outputs that mature in the first MaturityDelay blocks.
		if bn.height > types.MaturityDelay {
			// Sanity check - the map being deleted should be empty.
			if build.DEBUG {
				if len(cs.delayedSiacoinOutputs[bn.height]) != 0 {
					panic(errDeletingNonEmptyDelayedMap)
				}
			}
			delete(cs.delayedSiacoinOutputs, bn.height)
		}
	} else {
		// Sanity check - the map being deleted should be empty
		if build.DEBUG {
			if len(cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay]) != 0 {
				panic(errDeletingNonEmptyDelayedMap)
			}
		}
		delete(cs.delayedSiacoinOutputs, bn.height+types.MaturityDelay)
	}
}

// updateCurrentPath updates the current path after applying a diff set.
func (cs *ConsensusSet) updateCurrentPath(bn *blockNode, dir modules.DiffDirection) {
	// Update the current path.
	if dir == modules.DiffApply {
		cs.db.pushPath(bn.block.ID())
	} else {
		cs.db.popPath()
	}
}

// commitDiffSet applies or reverts the diffs in a blockNode.
func (cs *ConsensusSet) commitDiffSet(bn *blockNode, dir modules.DiffDirection) {
	cs.commitDiffSetSanity(bn, dir)
	cs.createUpcomingDelayedOutputMaps(bn, dir)
	cs.commitNodeDiffs(bn, dir)
	cs.deleteObsoleteDelayedOutputMaps(bn, dir)
	if cs.updatePath {
		cs.updateCurrentPath(bn, dir)
	}
}

// generateAndApplyDiff will verify the block and then integrate it into the
// consensus state. These two actions must happen at the same time because
// transactions are allowed to depend on each other. We can't be sure that a
// transaction is valid unless we have applied all of the previous transactions
// in the block, which means we need to apply while we verify.
func (cs *ConsensusSet) generateAndApplyDiff(bn *blockNode) error {
	// Sanity check
	if build.DEBUG {
		// Generate should only be called if the diffs have not yet been
		// generated.
		if bn.diffsGenerated {
			panic(errRegenerateDiffs)
		}

		// Current node must be the input node's parent.
		if bn.parent.block.ID() != cs.currentBlockID() {
			panic(errInvalidSuccessor)
		}
	}

	// Update the state to point to the new block.
	err := cs.db.addPath(bn.block.ID())
	if err != nil {
		return err
	}
	cs.delayedSiacoinOutputs[bn.height+types.MaturityDelay] = make(map[types.SiacoinOutputID]types.SiacoinOutput)

	// diffsGenerated is set to true as soon as we start changing the set of
	// diffs in the block node. If at any point the block is found to be
	// invalid, the diffs can be safely reversed from whatever point.
	bn.diffsGenerated = true

	// Validate and apply each transaction in the block. They cannot be
	// validated all at once because some transactions may not be valid until
	// previous transactions have been applied.
	for _, txn := range bn.block.Transactions {
		err := cs.validTransaction(txn)
		if err != nil {
			// Awkward: need to apply the matured outputs otherwise the diff
			// structure malforms due to the way the delayedOutput maps are
			// created and destroyed.
			cs.applyMaturedSiacoinOutputs(bn)
			cs.commitDiffSet(bn, modules.DiffRevert)
			cs.dosBlocks[bn.block.ID()] = struct{}{}
			cs.deleteNode(bn)
			return err
		}

		cs.applyTransaction(bn, txn)
	}

	// After all of the transactions have been applied, 'maintenance' is
	// applied on the block. This includes adding any outputs that have reached
	// maturity, applying any contracts with missed storage proofs, and adding
	// the miner payouts to the list of delayed outputs.
	cs.applyMaintenance(bn)

	if build.DEBUG {
		bn.consensusSetHash = cs.consensusSetHash()
	}
	return cs.db.addBlockMap(*bn)
}
