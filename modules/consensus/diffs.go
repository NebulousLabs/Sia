package consensus

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus/database"
)

var (
	errApplySiafundPoolDiffMismatch  = errors.New("committing a siafund pool diff with an invalid 'previous' field")
	errDiffsNotGenerated             = errors.New("applying diff set before generating errors")
	errInvalidSuccessor              = errors.New("generating diffs for a block that's an invalid successsor to the current block")
	errNegativePoolAdjustment        = errors.New("committing a siafund pool diff with a negative adjustment")
	errNonApplySiafundPoolDiff       = errors.New("committing a siafund pool diff that doesn't have the 'apply' direction")
	errRevertSiafundPoolDiffMismatch = errors.New("committing a siafund pool diff with an invalid 'adjusted' field")
	errWrongAppliedDiffSet           = errors.New("applying a diff set that isn't the current block")
	errWrongRevertDiffSet            = errors.New("reverting a diff set that isn't the current block")
)

// commitDiffSetSanity performs a series of sanity checks before committing a
// diff set.
func commitDiffSetSanity(tx database.Tx, b *database.Block, dir modules.DiffDirection) {
	// This function is purely sanity checks.
	if !build.DEBUG {
		return
	}

	// Diffs should have already been generated for this node.
	if !b.DiffsGenerated {
		panic(errDiffsNotGenerated)
	}

	// Current node must be the input node's parent if applying, and
	// current node must be the input node if reverting.
	if dir == modules.DiffApply {
		parent, err := getBlockMap(tx, b.ParentID)
		if build.DEBUG && err != nil {
			panic(err)
		}
		if parent.Block.ID() != currentBlockID(tx) {
			panic(errWrongAppliedDiffSet)
		}
	} else {
		if b.ID() != currentBlockID(tx) {
			panic(errWrongRevertDiffSet)
		}
	}
}

// commitSiacoinOutputDiff applies or reverts a SiacoinOutputDiff.
func commitSiacoinOutputDiff(tx database.Tx, scod modules.SiacoinOutputDiff, dir modules.DiffDirection) {
	if scod.Direction == dir {
		addSiacoinOutput(tx, scod.ID, scod.SiacoinOutput)
	} else {
		removeSiacoinOutput(tx, scod.ID)
	}
}

// commitFileContractDiff applies or reverts a FileContractDiff.
func commitFileContractDiff(tx database.Tx, fcd modules.FileContractDiff, dir modules.DiffDirection) {
	if fcd.Direction == dir {
		addFileContract(tx, fcd.ID, fcd.FileContract)
	} else {
		removeFileContract(tx, fcd.ID)
	}
}

// commitSiafundOutputDiff applies or reverts a Siafund output diff.
func commitSiafundOutputDiff(tx database.Tx, sfod modules.SiafundOutputDiff, dir modules.DiffDirection) {
	if sfod.Direction == dir {
		addSiafundOutput(tx, sfod.ID, sfod.SiafundOutput)
	} else {
		removeSiafundOutput(tx, sfod.ID)
	}
}

// commitDelayedSiacoinOutputDiff applies or reverts a delayedSiacoinOutputDiff.
func commitDelayedSiacoinOutputDiff(tx database.Tx, dscod modules.DelayedSiacoinOutputDiff, dir modules.DiffDirection) {
	if dscod.Direction == dir {
		addDSCO(tx, dscod.MaturityHeight, dscod.ID, dscod.SiacoinOutput)
	} else {
		removeDSCO(tx, dscod.MaturityHeight, dscod.ID)
	}
}

// commitSiafundPoolDiff applies or reverts a SiafundPoolDiff.
func commitSiafundPoolDiff(tx database.Tx, sfpd modules.SiafundPoolDiff, dir modules.DiffDirection) {
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
		if build.DEBUG && !getSiafundPool(tx).Equals(sfpd.Previous) {
			panic(errApplySiafundPoolDiffMismatch)
		}
		setSiafundPool(tx, sfpd.Adjusted)
	} else {
		// Sanity check - sfpd.Adjusted should equal the current siafund pool.
		if build.DEBUG && !getSiafundPool(tx).Equals(sfpd.Adjusted) {
			panic(errRevertSiafundPoolDiffMismatch)
		}
		setSiafundPool(tx, sfpd.Previous)
	}
}

// commitNodeDiffs commits all of the diffs in a block node.
func commitNodeDiffs(tx database.Tx, b *database.Block, dir modules.DiffDirection) {
	if dir == modules.DiffApply {
		for _, scod := range b.SiacoinOutputDiffs {
			commitSiacoinOutputDiff(tx, scod, dir)
		}
		for _, fcd := range b.FileContractDiffs {
			commitFileContractDiff(tx, fcd, dir)
		}
		for _, sfod := range b.SiafundOutputDiffs {
			commitSiafundOutputDiff(tx, sfod, dir)
		}
		for _, dscod := range b.DelayedSiacoinOutputDiffs {
			commitDelayedSiacoinOutputDiff(tx, dscod, dir)
		}
		for _, sfpd := range b.SiafundPoolDiffs {
			commitSiafundPoolDiff(tx, sfpd, dir)
		}
	} else {
		for i := len(b.SiacoinOutputDiffs) - 1; i >= 0; i-- {
			commitSiacoinOutputDiff(tx, b.SiacoinOutputDiffs[i], dir)
		}
		for i := len(b.FileContractDiffs) - 1; i >= 0; i-- {
			commitFileContractDiff(tx, b.FileContractDiffs[i], dir)
		}
		for i := len(b.SiafundOutputDiffs) - 1; i >= 0; i-- {
			commitSiafundOutputDiff(tx, b.SiafundOutputDiffs[i], dir)
		}
		for i := len(b.DelayedSiacoinOutputDiffs) - 1; i >= 0; i-- {
			commitDelayedSiacoinOutputDiff(tx, b.DelayedSiacoinOutputDiffs[i], dir)
		}
		for i := len(b.SiafundPoolDiffs) - 1; i >= 0; i-- {
			commitSiafundPoolDiff(tx, b.SiafundPoolDiffs[i], dir)
		}
	}
}

// updateCurrentPath updates the current path after applying a diff set.
func updateCurrentPath(tx database.Tx, b *database.Block, dir modules.DiffDirection) {
	// Update the current path.
	if dir == modules.DiffApply {
		pushPath(tx, b.ID())
	} else {
		popPath(tx)
	}
}

// commitDiffSet applies or reverts the diffs in a blockNode.
func commitDiffSet(tx database.Tx, b *database.Block, dir modules.DiffDirection) {
	// Sanity checks - there are a few so they were moved to another function.
	if build.DEBUG {
		commitDiffSetSanity(tx, b, dir)
	}

	commitNodeDiffs(tx, b, dir)
	updateCurrentPath(tx, b, dir)
}

// generateAndApplyDiff will verify the block and then integrate it into the
// consensus state. These two actions must happen at the same time because
// transactions are allowed to depend on each other. We can't be sure that a
// transaction is valid unless we have applied all of the previous transactions
// in the block, which means we need to apply while we verify.
func generateAndApplyDiff(tx database.Tx, b *database.Block) error {
	// Sanity check - the block being applied should have the current block as
	// a parent.
	if build.DEBUG && b.ParentID != currentBlockID(tx) {
		panic(errInvalidSuccessor)
	}

	// Validate and apply each transaction in the block. They cannot be
	// validated all at once because some transactions may not be valid until
	// previous transactions have been applied.
	for _, txn := range b.Transactions {
		err := validTransaction(tx, txn)
		if err != nil {
			return err
		}
		applyTransaction(tx, b, txn)
	}

	// After all of the transactions have been applied, 'maintenance' is
	// applied on the block. This includes adding any outputs that have reached
	// maturity, applying any contracts with missed storage proofs, and adding
	// the miner payouts to the list of delayed outputs.
	applyMaintenance(tx, b)

	// DiffsGenerated are only set to true after the block has been fully
	// validated and integrated. This is required to prevent later blocks from
	// being accepted on top of an invalid block - if the consensus set ever
	// forks over an invalid block, 'DiffsGenerated' will be set to 'false',
	// requiring validation to occur again. when 'DiffsGenerated' is set to
	// true, validation is skipped, therefore the flag should only be set to
	// true on fully validated blocks.
	b.DiffsGenerated = true

	// Add the block to the current path and block map.
	bid := b.ID()
	blockMap := tx.Bucket(BlockMap)
	updateCurrentPath(tx, b, modules.DiffApply)

	// Sanity check preparation - set the consensus hash at this height so that
	// during reverting a check can be performed to assure consistency when
	// adding and removing blocks. Must happen after the block is added to the
	// path.
	if build.DEBUG {
		b.ConsensusChecksum = tx.ConsensusChecksum()
	}

	return blockMap.Put(bid[:], encoding.Marshal(*b))
}
