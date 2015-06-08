package consensus

import (
	"errors"
	"math/big"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrInvalidStorageProof                = errors.New("provided storage proof is invalid")
	ErrLowRevisionNumber                  = errors.New("transaction has a file contract with an outdated revision number")
	ErrMissingSiacoinOutput               = errors.New("transaction spends a nonexisting siacoin output")
	ErrMissingFileContract                = errors.New("transaction terminates a nonexisting file contract")
	ErrMissingSiafundOutput               = errors.New("transaction spends a nonexisting siafund output")
	ErrSiacoinInputOutputMismatch         = errors.New("siacoin inputs do not equal siacoin outputs for transaction")
	ErrUnfinishedFileContract             = errors.New("file contract window has not yet openend")
	ErrUnrecognizedFileContractID         = errors.New("cannot fetch storage proof segment for unknown file contract")
	ErrWrongSiacoinOutputUnlockConditions = errors.New("transaction contains a siacoin output with incorrect unlock conditions")
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
			return ErrWrongSiacoinOutputUnlockConditions
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
		fc, exists := cs.fileContracts[sp.ParentID]
		if !exists {
			return errors.New("unrecognized file contract ID in storage proof")
		}

		// Check that the storage proof itself is valid.
		segmentIndex, err := cs.storageProofSegment(sp.ParentID)
		if err != nil {
			return err
		}

		verified := crypto.VerifySegment(
			sp.Segment,
			sp.HashSet,
			crypto.CalculateLeaves(fc.FileSize),
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
			return ErrMissingFileContract
		}

		// Check that the height is less than fc.WindowStart - revisions are
		// not allowed to be submitted once the storage proof window has
		// opened.  This reduces complexity for unconfirmed transactions.
		if cs.height() > fc.WindowStart {
			return errors.New("contract revision submitted too late")
		}

		// Check that the revision number of the revision is greater than the
		// revision number of the existing file contract.
		if fc.RevisionNumber >= fcr.NewRevisionNumber {
			return ErrLowRevisionNumber
		}

		// Check that the unlock conditions match the unlock hash.
		if fcr.UnlockConditions.UnlockHash() != fc.UnlockHash {
			return errors.New("unlock conditions don't match unlock hash")
		}

		// Check that the payout of the revision matches the payout of the
		// original.
		//
		// txn.StandaloneValid checks for the validity of the
		// ValidProofOutputs.
		var payout types.Currency
		for _, output := range fcr.NewMissedProofOutputs {
			payout = payout.Add(output.Value)
		}
		if payout.Cmp(fc.Payout) != 0 {
			return errors.New("contract revision has incorrect payouts")
		}
	}

	return
}

// validSiafunds checks that the siafund portions of the transaction are valid
// in the context of the consensus set.
func (s *State) validSiafunds(t types.Transaction) (err error) {
	// Compare the number of input siafunds to the output siafunds.
	var siafundInputSum types.Currency
	var siafundOutputSum types.Currency
	for _, sfi := range t.SiafundInputs {
		sfo, exists := s.siafundOutputs[sfi.ParentID]
		if !exists {
			return ErrMissingSiafundOutput
		}

		// Check the unlock conditions match the unlock hash.
		if sfi.UnlockConditions.UnlockHash() != sfo.UnlockHash {
			return errors.New("unlock conditions don't match required unlock hash")
		}

		siafundInputSum = siafundInputSum.Add(sfo.Value)
	}
	for _, sfo := range t.SiafundOutputs {
		siafundOutputSum = siafundOutputSum.Add(sfo.Value)
	}
	if siafundOutputSum.Cmp(siafundInputSum) != 0 {
		return errors.New("siafund inputs do not equal siafund outpus within transaction")
	}
	return
}

// ValidStorageProofs checks that the storage proofs are valid in the context
// of the consensus set.
func (s *State) ValidStorageProofs(t types.Transaction) (err error) {
	id := s.mu.RLock()
	defer s.mu.RUnlock(id)
	return s.validStorageProofs(t)
}

// validTransaction checks that all fields are valid within the current
// consensus state. If not an error is returned.
func (s *State) validTransaction(t types.Transaction) error {
	// Skip transaction verification if the State is accepting trusted blocks.
	if s.verificationRigor != fullVerification {
		return nil
	}

	// StandaloneValid will check things like signatures and properties that
	// should be inherent to the transaction. (storage proof rules, etc.)
	err := t.StandaloneValid(s.height())
	if err != nil {
		return err
	}

	// Check that each portion of the transaction is legal given the current
	// consensus set.
	err = s.validSiacoins(t)
	if err != nil {
		return err
	}
	err = s.validStorageProofs(t)
	if err != nil {
		return err
	}
	err = s.validFileContractRevisions(t)
	if err != nil {
		return err
	}
	err = s.validSiafunds(t)
	if err != nil {
		return err
	}

	return nil
}

// TryTransactions applies the input transactions to the consensus set to
// determine if they are valid. An error is returned IFF they are not a valid
// set in the current consensus set. The size of the transactions and the set
// is not checked.
func (s *State) TryTransactions(txns []types.Transaction) error {
	// applyTransaction will apply the diffs from a transaction and store them
	// in a block node. diffHolder is the blockNode that tracks the temporary
	// changes. At the end of the function, all changes that were made to the
	// consensus set get reverted.
	var diffHolder *blockNode
	defer s.commitDiffSet(diffHolder, modules.DiffRevert)

	for _, txn := range txns {
		err := s.validTransaction(txn)
		if err != nil {
			return err
		}
		s.applyTransaction(diffHolder, txn)
	}

	return nil
}
