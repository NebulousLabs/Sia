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
func (cs *ConsensusSet) commitDiffSetSanity(pb *processedBlock, dir modules.DiffDirection) {
	// Sanity checks.
	if build.DEBUG {
		// Diffs should have already been generated for this node.
		if !pb.DiffsGenerated {
			panic(errDiffsNotGenerated)
		}

		// Current node must be the input node's parent if applying, and
		// current node must be the input node if reverting.
		if dir == modules.DiffApply {
			parent := cs.db.getBlockMap(pb.Parent)
			if parent.Block.ID() != cs.currentBlockID() {
				panic(errWrongAppliedDiffSet)
			}
		} else {
			if pb.Block.ID() != cs.currentBlockID() {
				panic(errWrongRevertDiffSet)
			}
		}
	}
}

// createUpcomingDelayeOutputdMaps creates the delayed siacoin output maps that
// will be used when applying delayed siacoin outputs in the diff set.
func (cs *ConsensusSet) createUpcomingDelayedOutputMaps(pb *processedBlock, dir modules.DiffDirection) {
	if dir == modules.DiffApply {
		if build.DEBUG {
			// Sanity check - the output map being created should not already
			// exist.
			_, exists := cs.delayedSiacoinOutputs[pb.Height+types.MaturityDelay]
			if exists {
				panic(errCreatingExistingUpcomingMap)
			}
		}
		cs.delayedSiacoinOutputs[pb.Height+types.MaturityDelay] = make(map[types.SiacoinOutputID]types.SiacoinOutput)
	} else {
		// Skip creating maps for heights that can't have delayed outputs.
		if pb.Height > types.MaturityDelay {
			// Sanity check - the output map being created should not already
			// exist.
			if build.DEBUG {
				_, exists := cs.delayedSiacoinOutputs[pb.Height]
				if exists {
					panic(errCreatingExistingUpcomingMap)
				}
			}
			cs.delayedSiacoinOutputs[pb.Height] = make(map[types.SiacoinOutputID]types.SiacoinOutput)
		}
	}
}

// commitNodeDiffs commits all of the diffs in a block node.
func (cs *ConsensusSet) commitNodeDiffs(pb *processedBlock, dir modules.DiffDirection) {
	if dir == modules.DiffApply {
		for _, scod := range pb.SiacoinOutputDiffs {
			cs.commitSiacoinOutputDiff(scod, dir)
		}
		for _, fcd := range pb.FileContractDiffs {
			cs.commitFileContractDiff(fcd, dir)
		}
		for _, sfod := range pb.SiafundOutputDiffs {
			cs.commitSiafundOutputDiff(sfod, dir)
		}
		for _, dscod := range pb.DelayedSiacoinOutputDiffs {
			cs.commitDelayedSiacoinOutputDiff(dscod, dir)
		}
		for _, sfpd := range pb.SiafundPoolDiffs {
			cs.commitSiafundPoolDiff(sfpd, dir)
		}
	} else {
		for i := len(pb.SiacoinOutputDiffs) - 1; i >= 0; i-- {
			cs.commitSiacoinOutputDiff(pb.SiacoinOutputDiffs[i], dir)
		}
		for i := len(pb.FileContractDiffs) - 1; i >= 0; i-- {
			cs.commitFileContractDiff(pb.FileContractDiffs[i], dir)
		}
		for i := len(pb.SiafundOutputDiffs) - 1; i >= 0; i-- {
			cs.commitSiafundOutputDiff(pb.SiafundOutputDiffs[i], dir)
		}
		for i := len(pb.DelayedSiacoinOutputDiffs) - 1; i >= 0; i-- {
			cs.commitDelayedSiacoinOutputDiff(pb.DelayedSiacoinOutputDiffs[i], dir)
		}
		for i := len(pb.SiafundPoolDiffs) - 1; i >= 0; i-- {
			cs.commitSiafundPoolDiff(pb.SiafundPoolDiffs[i], dir)
		}
	}
}

// deleteObsoleteDelayedOutputMaps deletes the delayed siacoin output maps that
// are no longer in use.
func (cs *ConsensusSet) deleteObsoleteDelayedOutputMaps(pb *processedBlock, dir modules.DiffDirection) {
	if dir == modules.DiffApply {
		// There are no outputs that mature in the first MaturityDelay blocks.
		if pb.Height > types.MaturityDelay {
			// Sanity check - the map being deleted should be empty.
			if build.DEBUG {
				if len(cs.delayedSiacoinOutputs[pb.Height]) != 0 {
					panic(errDeletingNonEmptyDelayedMap)
				}
			}
			delete(cs.delayedSiacoinOutputs, pb.Height)
		}
	} else {
		// Sanity check - the map being deleted should be empty
		if build.DEBUG {
			if len(cs.delayedSiacoinOutputs[pb.Height+types.MaturityDelay]) != 0 {
				panic(errDeletingNonEmptyDelayedMap)
			}
		}
		delete(cs.delayedSiacoinOutputs, pb.Height+types.MaturityDelay)
	}
}

// updateCurrentPath updates the current path after applying a diff set.
func (cs *ConsensusSet) updateCurrentPath(pb *processedBlock, dir modules.DiffDirection) {
	// Update the current path.
	if dir == modules.DiffApply {
		if cs.updatePath {
			err := cs.db.pushPath(pb.Block.ID())

			if build.DEBUG && err != nil {
				panic(err)
			}
		}
		cs.blocksLoaded += 1
	} else {
		err := cs.db.popPath()
		if build.DEBUG && err != nil {
			panic(err)
		}
		cs.blocksLoaded -= 1
	}
}

// commitDiffSet applies or reverts the diffs in a blockNode.
func (cs *ConsensusSet) commitDiffSet(pb *processedBlock, dir modules.DiffDirection) {
	cs.commitDiffSetSanity(pb, dir)
	cs.createUpcomingDelayedOutputMaps(pb, dir)
	cs.commitNodeDiffs(pb, dir)
	cs.deleteObsoleteDelayedOutputMaps(pb, dir)
	cs.updateCurrentPath(pb, dir)
}

// generateAndApplyDiff will verify the block and then integrate it into the
// consensus state. These two actions must happen at the same time because
// transactions are allowed to depend on each other. We can't be sure that a
// transaction is valid unless we have applied all of the previous transactions
// in the block, which means we need to apply while we verify.
func (cs *ConsensusSet) generateAndApplyDiff(pb *processedBlock) error {
	// Sanity check
	if build.DEBUG {
		// Generate should only be called if the diffs have not yet been
		// generated.
		if pb.DiffsGenerated {
			panic(errRegenerateDiffs)
		}

		// Current node must be the input node's parent.
		parent := cs.db.getBlockMap(pb.Parent)
		if parent.Block.ID() != cs.currentBlockID() {
			panic(errInvalidSuccessor)
		}
	}

	// Update the state to point to the new block.
	err := cs.db.pushPath(pb.Block.ID())
	if err != nil {
		return err
	}
	cs.blocksLoaded += 1
	cs.delayedSiacoinOutputs[pb.Height+types.MaturityDelay] = make(map[types.SiacoinOutputID]types.SiacoinOutput)

	// diffsGenerated is set to true as soon as we start changing the set of
	// diffs in the block node. If at any point the block is found to be
	// invalid, the diffs can be safely reversed from whatever point.
	pb.DiffsGenerated = true

	// Validate and apply each transaction in the block. They cannot be
	// validated all at once because some transactions may not be valid until
	// previous transactions have been applied.
	for _, txn := range pb.Block.Transactions {
		err := cs.validTransaction(txn)
		if err != nil {
			// Awkward: need to apply the matured outputs otherwise the diff
			// structure malforms due to the way the delayedOutput maps are
			// created and destroyed.
			cs.applyMaturedSiacoinOutputs(pb)
			cs.commitDiffSet(pb, modules.DiffRevert)
			cs.dosBlocks[pb.Block.ID()] = struct{}{}
			bn := cs.pbToBn(pb)
			cs.deleteNode(&bn)
			return err
		}

		bn := cs.pbToBn(pb)
		cs.applyTransaction(&bn, txn)
	}

	// After all of the transactions have been applied, 'maintenance' is
	// applied on the block. This includes adding any outputs that have reached
	// maturity, applying any contracts with missed storage proofs, and adding
	// the miner payouts to the list of delayed outputs.
	cs.applyMaintenance(pb)

	if build.DEBUG {
		pb.ConsensusSetHash = cs.consensusSetHash()
	}

	// Replace the unprocessed block in the block map with a processed one
	err = cs.db.rmBlockMap(pb.Block.ID())
	if err != nil {
		return err
	}

	return cs.db.addBlockMap(*pb)
}
