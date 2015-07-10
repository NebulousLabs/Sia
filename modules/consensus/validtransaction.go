package consensus

import (
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrAlteredRevisionPayouts     = errors.New("file contract revision has altered payout volume")
	ErrInvalidStorageProof        = errors.New("provided storage proof is invalid")
	ErrLateRevision               = errors.New("file contract revision submitted after deadline")
	ErrLowRevisionNumber          = errors.New("transaction has a file contract with an outdated revision number")
	ErrMissingSiacoinOutput       = errors.New("transaction spends a nonexisting siacoin output")
	ErrMissingSiafundOutput       = errors.New("transaction spends a nonexisting siafund output")
	ErrSiacoinInputOutputMismatch = errors.New("siacoin inputs do not equal siacoin outputs for transaction")
	ErrSiafundInputOutputMismatch = errors.New("siafund inputs do not equal siafund outputs for transaction")
	ErrUnfinishedFileContract     = errors.New("file contract window has not yet openend")
	ErrUnrecognizedFileContractID = errors.New("cannot fetch storage proof segment for unknown file contract")
	ErrWrongUnlockConditions      = errors.New("transaction contains incorrect unlock conditions")
)

// validSiacoins checks that the siacoin inputs and outputs are valid in the
// context of the current consensus set.
func (cs *State) validSiacoins(t types.Transaction) (err error) {
	var inputSum types.Currency
	for _, sci := range t.SiacoinInputs {
		// Check that the input spends an existing output.
		sco, exists := cs.siacoinOutputs[sci.ParentID]
		if !exists {
			return ErrMissingSiacoinOutput
		}

		// Check that the unlock conditions match the required unlock hash.
		if sci.UnlockConditions.UnlockHash() != sco.UnlockHash {
			return ErrWrongUnlockConditions
		}

		inputSum = inputSum.Add(sco.Value)
	}
	if inputSum.Cmp(t.SiacoinOutputSum()) != 0 {
		return ErrSiacoinInputOutputMismatch
	}
	return
}

// storageProofSegment returns the index of the segment that needs to be proven
// exists in a file contract.
func (cs *State) storageProofSegment(fcid types.FileContractID) (index uint64, err error) {
	// Get the file contract associated with the input id.
	fc, exists := cs.fileContracts[fcid]
	if !exists {
		return 0, ErrUnrecognizedFileContractID
	}

	// Get the ID of the trigger block.
	triggerHeight := fc.WindowStart - 1
	if triggerHeight > cs.height() {
		return 0, ErrUnfinishedFileContract
	}
	triggerID := cs.currentPath[triggerHeight]

	// Get the index by appending the file contract ID to the trigger block and
	// taking the hash, then converting the hash to a numerical value and
	// modding it against the number of segments in the file. The result is a
	// random number in range [0, numSegments]. The probability is very
	// slightly weighted towards the beginning of the file, but because the
	// size difference between the number of segments and the random number
	// being modded, the difference is too small to make any practical
	// difference.
	seed := crypto.HashAll(triggerID, fcid)
	numSegments := int64(crypto.CalculateLeaves(fc.FileSize))
	seedInt := new(big.Int).SetBytes(seed[:])
	index = seedInt.Mod(seedInt, big.NewInt(numSegments)).Uint64()
	return index, nil
}

// validStorageProofs checks that the storage proofs are valid in the context
// of the consensus set.
func (cs *State) validStorageProofs(t types.Transaction) error {
	for _, sp := range t.StorageProofs {
		// Check that the storage proof itself is valid.
		segmentIndex, err := cs.storageProofSegment(sp.ParentID)
		if err != nil {
			return err
		}

		fc, _ := cs.fileContracts[sp.ParentID] // previous function verifies the file contract exists
		leaves := crypto.CalculateLeaves(fc.FileSize)
		segmentLen := uint64(crypto.SegmentSize)
		if segmentIndex == leaves-1 {
			segmentLen = fc.FileSize % crypto.SegmentSize
		}

		// COMPATv0.4.0
		//
		// Fixing the padding situation resulted in a hardfork. The below code
		// will stop the hardfork from triggering before block 15,000.
		types.CurrentHeightLock.Lock()
		if (build.Release == "standard" && types.CurrentHeight < 15e3) || (build.Release == "testing" && types.CurrentHeight < 10) {
			segmentLen = uint64(crypto.SegmentSize)
		}
		types.CurrentHeightLock.Unlock()

		verified := crypto.VerifySegment(
			sp.Segment[:segmentLen],
			sp.HashSet,
			leaves,
			segmentIndex,
			fc.FileMerkleRoot,
		)
		if !verified {
			return ErrInvalidStorageProof
		}
	}

	return nil
}

// validFileContractRevision checks that each file contract revision is valid
// in the context of the current consensus set.
func (cs *State) validFileContractRevisions(t types.Transaction) (err error) {
	for _, fcr := range t.FileContractRevisions {
		// Check that the revision revises an existing contract.
		fc, exists := cs.fileContracts[fcr.ParentID]
		if !exists {
			return ErrUnrecognizedFileContractID
		}

		// Check that the height is less than fc.WindowStart - revisions are
		// not allowed to be submitted once the storage proof window has
		// opened.  This reduces complexity for unconfirmed transactions.
		if cs.height() > fc.WindowStart {
			return ErrLateRevision
		}

		// Check that the revision number of the revision is greater than the
		// revision number of the existing file contract.
		if fc.RevisionNumber >= fcr.NewRevisionNumber {
			return ErrLowRevisionNumber
		}

		// Check that the unlock conditions match the unlock hash.
		if fcr.UnlockConditions.UnlockHash() != fc.UnlockHash {
			return ErrWrongUnlockConditions
		}

		// Check that the payout of the revision matches the payout of the
		// original, and that the payouts match eachother.
		var validPayout, missedPayout types.Currency
		for _, output := range fcr.NewValidProofOutputs {
			validPayout = validPayout.Add(output.Value)
		}
		for _, output := range fcr.NewMissedProofOutputs {
			missedPayout = missedPayout.Add(output.Value)
		}
		if validPayout.Cmp(fc.Payout.Sub(fc.Tax())) != 0 {
			return ErrAlteredRevisionPayouts
		}
		if missedPayout.Cmp(fc.Payout.Sub(fc.Tax())) != 0 {
			return ErrAlteredRevisionPayouts
		}
	}

	return
}

// validSiafunds checks that the siafund portions of the transaction are valid
// in the context of the consensus set.
func (cs *State) validSiafunds(t types.Transaction) (err error) {
	// Compare the number of input siafunds to the output siafunds.
	var siafundInputSum types.Currency
	var siafundOutputSum types.Currency
	for _, sfi := range t.SiafundInputs {
		sfo, exists := cs.siafundOutputs[sfi.ParentID]
		if !exists {
			return ErrMissingSiafundOutput
		}

		// Check the unlock conditions match the unlock hash.
		if sfi.UnlockConditions.UnlockHash() != sfo.UnlockHash {
			return ErrWrongUnlockConditions
		}

		siafundInputSum = siafundInputSum.Add(sfo.Value)
	}
	for _, sfo := range t.SiafundOutputs {
		siafundOutputSum = siafundOutputSum.Add(sfo.Value)
	}
	if siafundOutputSum.Cmp(siafundInputSum) != 0 {
		return ErrSiafundInputOutputMismatch
	}
	return
}

// ValidStorageProofs checks that the storage proofs are valid in the context
// of the consensus set.
func (cs *State) ValidStorageProofs(t types.Transaction) (err error) {
	id := cs.mu.RLock()
	defer cs.mu.RUnlock(id)
	return cs.validStorageProofs(t)
}

// validTransaction checks that all fields are valid within the current
// consensus state. If not an error is returned.
func (cs *State) validTransaction(t types.Transaction) error {
	// Skip transaction verification if the State is accepting trusted blocks.
	if cs.verificationRigor != fullVerification {
		return nil
	}

	// StandaloneValid will check things like signatures and properties that
	// should be inherent to the transaction. (storage proof rules, etc.)
	err := t.StandaloneValid(cs.height())
	if err != nil {
		return err
	}

	// Check that each portion of the transaction is legal given the current
	// consensus set.
	err = cs.validSiacoins(t)
	if err != nil {
		return err
	}
	err = cs.validStorageProofs(t)
	if err != nil {
		return err
	}
	err = cs.validFileContractRevisions(t)
	if err != nil {
		return err
	}
	err = cs.validSiafunds(t)
	if err != nil {
		return err
	}

	return nil
}

// TryTransactions applies the input transactions to the consensus set to
// determine if they are valid. An error is returned IFF they are not a valid
// set in the current consensus set. The size of the transactions and the set
// is not checked.
func (cs *State) TryTransactions(txns []types.Transaction) error {
	// applyTransaction will apply the diffs from a transaction and store them
	// in a block node. diffHolder is the blockNode that tracks the temporary
	// changes. At the end of the function, all changes that were made to the
	// consensus set get reverted.
	diffHolder := new(blockNode)
	defer cs.commitNodeDiffs(diffHolder, modules.DiffRevert)
	for _, txn := range txns {
		err := cs.validTransaction(txn)
		if err != nil {
			return err
		}
		cs.applyTransaction(diffHolder, txn)
	}
	return nil
}
