package consensus

import (
	"errors"
	"math/big"

	"github.com/boltdb/bolt"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
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

	// Boltdb will only roll back a tx if an error is returned. In the case of
	// TryTransactionSet, we want to roll back the tx even if there is no
	// error. So errSuccess is returned. An alternate method would be to
	// manually manage the tx instead of using 'Update', but that has safety
	// concerns and is more difficult to implement correctly.
	errSuccess = errors.New("please")
)

// validTxSiacoins checks that the siacoin inputs and outputs are valid in the
// context of the current consensus set.
func (cs *ConsensusSet) validTxSiacoins(tx *bolt.Tx, t types.Transaction) error {
	scoBucket := tx.Bucket(SiacoinOutputs)
	var inputSum types.Currency
	for _, sci := range t.SiacoinInputs {
		// Check that the input spends an existing output.
		scoBytes := scoBucket.Get(sci.ParentID[:])
		if scoBytes == nil {
			return ErrMissingSiacoinOutput
		}

		// Check that the unlock conditions match the required unlock hash.
		var sco types.SiacoinOutput
		err := encoding.Unmarshal(scoBytes, &sco)
		if build.DEBUG && err != nil {
			panic(err)
		}
		if sci.UnlockConditions.UnlockHash() != sco.UnlockHash {
			return ErrWrongUnlockConditions
		}

		inputSum = inputSum.Add(sco.Value)
	}
	if inputSum.Cmp(t.SiacoinOutputSum()) != 0 {
		return ErrSiacoinInputOutputMismatch
	}
	return nil
}

// validSiacoins checks that the siacoin inputs and outputs are valid in the
// context of the current consensus set.
func (cs *ConsensusSet) validSiacoins(t types.Transaction) error {
	return cs.db.View(func(tx *bolt.Tx) error {
		scoBucket := tx.Bucket(SiacoinOutputs)
		var inputSum types.Currency
		for _, sci := range t.SiacoinInputs {
			// Check that the input spends an existing output.
			scoBytes := scoBucket.Get(sci.ParentID[:])
			if scoBytes == nil {
				return ErrMissingSiacoinOutput
			}

			// Check that the unlock conditions match the required unlock hash.
			var sco types.SiacoinOutput
			err := encoding.Unmarshal(scoBytes, &sco)
			if build.DEBUG && err != nil {
				panic(err)
			}
			if sci.UnlockConditions.UnlockHash() != sco.UnlockHash {
				return ErrWrongUnlockConditions
			}

			inputSum = inputSum.Add(sco.Value)
		}
		if inputSum.Cmp(t.SiacoinOutputSum()) != 0 {
			return ErrSiacoinInputOutputMismatch
		}
		return nil
	})
}

// txStorageProofSegment returns the index of the segment that needs to be proven
// exists in a file contract.
func (cs *ConsensusSet) txStorageProofSegment(tx *bolt.Tx, fcid types.FileContractID) (uint64, error) {
	// Check that the parent file contract exists.
	fcBucket := tx.Bucket(FileContracts)
	fcBytes := fcBucket.Get(fcid[:])
	if fcBytes == nil {
		return 0, ErrUnrecognizedFileContractID
	}

	// Decode the file contract.
	var fc types.FileContract
	err := encoding.Unmarshal(fcBytes, &fc)
	if build.DEBUG && err != nil {
		panic(err)
	}

	// Get the trigger block id.
	blockPath := tx.Bucket(BlockPath)
	triggerHeight := fc.WindowStart - 1
	if triggerHeight > types.BlockHeight(blockPath.Stats().KeyN) {
		return 0, ErrUnfinishedFileContract
	}
	var triggerID types.BlockID
	copy(triggerID[:], blockPath.Get(encoding.EncUint64(uint64(triggerHeight))))

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
	index := seedInt.Mod(seedInt, big.NewInt(numSegments)).Uint64()
	return index, nil
}

// storageProofSegment returns the index of the segment that needs to be proven
// exists in a file contract.
func (cs *ConsensusSet) storageProofSegment(fcid types.FileContractID) (index uint64, err error) {
	err = cs.db.View(func(tx *bolt.Tx) error {
		// Check that the parent file contract exists.
		fcBucket := tx.Bucket(FileContracts)
		fcBytes := fcBucket.Get(fcid[:])
		if fcBytes == nil {
			return ErrUnrecognizedFileContractID
		}

		// Decode the file contract.
		var fc types.FileContract
		err := encoding.Unmarshal(fcBytes, &fc)
		if build.DEBUG && err != nil {
			panic(err)
		}

		// Get the trigger block id.
		blockPath := tx.Bucket(BlockPath)
		triggerHeight := fc.WindowStart - 1
		if triggerHeight > types.BlockHeight(blockPath.Stats().KeyN) {
			return ErrUnfinishedFileContract
		}
		var triggerID types.BlockID
		copy(triggerID[:], blockPath.Get(encoding.EncUint64(uint64(triggerHeight))))

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
		return nil
	})
	if err != nil {
		return 0, err
	}
	return index, nil
}

// validTxStorageProofs checks that the storage proofs are valid in the context
// of the consensus set.
func (cs *ConsensusSet) validTxStorageProofs(tx *bolt.Tx, t types.Transaction) error {
	for _, sp := range t.StorageProofs {
		// Check that the storage proof itself is valid.
		segmentIndex, err := cs.txStorageProofSegment(tx, sp.ParentID)
		if err != nil {
			return err
		}

		fc, err := getFileContract(tx, sp.ParentID)
		if err != nil {
			return err
		}
		leaves := crypto.CalculateLeaves(fc.FileSize)
		segmentLen := uint64(crypto.SegmentSize)
		if segmentIndex == leaves-1 {
			segmentLen = fc.FileSize % crypto.SegmentSize
		}

		// COMPATv0.4.0
		//
		// Fixing the padding situation resulted in a hardfork. The below code
		// will stop the hardfork from triggering before block 20,000.
		types.CurrentHeightLock.Lock()
		if (build.Release == "standard" && types.CurrentHeight < 21e3) || (build.Release == "testing" && types.CurrentHeight < 10) {
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

// validStorageProofs checks that the storage proofs are valid in the context
// of the consensus set.
func (cs *ConsensusSet) validStorageProofs(t types.Transaction) error {
	for _, sp := range t.StorageProofs {
		// Check that the storage proof itself is valid.
		segmentIndex, err := cs.storageProofSegment(sp.ParentID)
		if err != nil {
			return err
		}

		fc := cs.db.getFileContracts(sp.ParentID) // previous function verifies the file contract exists
		leaves := crypto.CalculateLeaves(fc.FileSize)
		segmentLen := uint64(crypto.SegmentSize)
		if segmentIndex == leaves-1 {
			segmentLen = fc.FileSize % crypto.SegmentSize
		}

		// COMPATv0.4.0
		//
		// Fixing the padding situation resulted in a hardfork. The below code
		// will stop the hardfork from triggering before block 20,000.
		types.CurrentHeightLock.Lock()
		if (build.Release == "standard" && types.CurrentHeight < 21e3) || (build.Release == "testing" && types.CurrentHeight < 10) {
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

// validTxFileContractRevision checks that each file contract revision is valid
// in the context of the current consensus set.
func (cs *ConsensusSet) validTxFileContractRevisions(tx *bolt.Tx, t types.Transaction) error {
	for _, fcr := range t.FileContractRevisions {
		fc, err := getFileContract(tx, fcr.ParentID)
		if err != nil {
			return err
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
	return nil
}

// validFileContractRevision checks that each file contract revision is valid
// in the context of the current consensus set.
func (cs *ConsensusSet) validFileContractRevisions(t types.Transaction) (err error) {
	for _, fcr := range t.FileContractRevisions {
		// Check that the revision revises an existing contract.
		exists := cs.db.inFileContracts(fcr.ParentID)
		if !exists {
			return ErrUnrecognizedFileContractID
		}
		fc := cs.db.getFileContracts(fcr.ParentID)

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

// validTxSiafunds checks that the siafund portions of the transaction are valid
// in the context of the consensus set.
func (cs *ConsensusSet) validTxSiafunds(tx *bolt.Tx, t types.Transaction) (err error) {
	// Compare the number of input siafunds to the output siafunds.
	var siafundInputSum types.Currency
	var siafundOutputSum types.Currency
	for _, sfi := range t.SiafundInputs {
		sfo, err := getSiafundOutput(tx, sfi.ParentID)
		if err != nil {
			return err
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

// validSiafunds checks that the siafund portions of the transaction are valid
// in the context of the consensus set.
func (cs *ConsensusSet) validSiafunds(t types.Transaction) (err error) {
	// Compare the number of input siafunds to the output siafunds.
	var siafundInputSum types.Currency
	var siafundOutputSum types.Currency
	for _, sfi := range t.SiafundInputs {
		exists := cs.db.inSiafundOutputs(sfi.ParentID)
		if !exists {
			return ErrMissingSiafundOutput
		}
		sfo := cs.db.getSiafundOutputs(sfi.ParentID)

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
func (cs *ConsensusSet) ValidStorageProofs(t types.Transaction) (err error) {
	id := cs.mu.RLock()
	defer cs.mu.RUnlock(id)
	return cs.validStorageProofs(t)
}

// validTxTransaction checks that all fields are valid within the current
// consensus state. If not an error is returned.
func (cs *ConsensusSet) validTxTransaction(tx *bolt.Tx, t types.Transaction) error {
	// StandaloneValid will check things like signatures and properties that
	// should be inherent to the transaction. (storage proof rules, etc.)
	err := t.StandaloneValid(cs.height())
	if err != nil {
		return err
	}

	// Check that each portion of the transaction is legal given the current
	// consensus set.
	err = cs.validTxSiacoins(tx, t)
	if err != nil {
		return err
	}
	err = cs.validTxStorageProofs(tx, t)
	if err != nil {
		return err
	}
	err = cs.validTxFileContractRevisions(tx, t)
	if err != nil {
		return err
	}
	err = cs.validTxSiafunds(tx, t)
	if err != nil {
		return err
	}
	return nil
}

// validTransaction checks that all fields are valid within the current
// consensus state. If not an error is returned.
func (cs *ConsensusSet) validTransaction(t types.Transaction) error {
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

// TryTransactionSet applies the input transactions to the consensus set to
// determine if they are valid. An error is returned IFF they are not a valid
// set in the current consensus set. The size of the transactions and the set
// is not checked. After the transactions have been validated, a consensus
// change is returned detailing the diffs that the transaciton set would have.
func (cs *ConsensusSet) TryTransactionSet(txns []types.Transaction) (modules.ConsensusChange, error) {
	// applyTransaction will apply the diffs from a transaction and store them
	// in a block node. diffHolder is the blockNode that tracks the temporary
	// changes. At the end of the function, all changes that were made to the
	// consensus set get reverted.
	diffHolder := new(processedBlock)
	diffHolder.Height = cs.height()

	// Grab the old siafund pool so it can be reverted after TryTransactionSet.
	sfpOld := cs.siafundPool
	err := cs.db.Update(func(tx *bolt.Tx) error {
		for _, txn := range txns {
			err := cs.validTxTransaction(tx, txn)
			if err != nil {
				return err
			}
			err = cs.applyTransaction(tx, diffHolder, txn)
			if err != nil {
				return err
			}
		}
		return errSuccess
	})
	// Revert the siafund pool before checking the error.
	cs.siafundPool = sfpOld
	if err != errSuccess {
		return modules.ConsensusChange{}, err
	}
	cc := modules.ConsensusChange{
		SiacoinOutputDiffs:        diffHolder.SiacoinOutputDiffs,
		FileContractDiffs:         diffHolder.FileContractDiffs,
		SiafundOutputDiffs:        diffHolder.SiafundOutputDiffs,
		DelayedSiacoinOutputDiffs: diffHolder.DelayedSiacoinOutputDiffs,
		SiafundPoolDiffs:          diffHolder.SiafundPoolDiffs,
	}
	return cc, nil
}
