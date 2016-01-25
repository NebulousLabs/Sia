package host

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// resubmissionTimeout is the number of blocks that the host will wait
	// before trying to resubmit a transaction to the blockchain.
	resubmissionTimeout = 2

	// Booleans to indicate that a contract obligation has been successful or
	// unsuccessful.
	obligationSucceeded = true
	obligationFailed    = false
)

var (
	// confirmationRequirement is the number of blocks that the host waits
	// before assuming that a storage proof has been confirmed by the
	// blockchain, and will not need to be reconstructed.
	confirmationRequirement = func() types.BlockHeight {
		if build.Release == "testing" {
			return 3
		}
		if build.Release == "standard" {
			return 12
		}
		if build.Release == "dev" {
			return 6
		}
		panic("unrecognized release value")
	}()
)

// A contractObligation tracks a file contract that the host is obligated to
// fulfill.
type contractObligation struct {
	// Blockchain tracking. The storage proof transaction is not tracked
	// because the same storage proof transaction is not always guaranteed to
	// be valid. If the origin or the revision has not been confirmed, the host
	// will need to resubmit them to the transaction pool.
	ID                  types.FileContractID // The ID of the file contract.
	OriginTransaction   types.Transaction    // The transaction containing the original file contract.
	RevisionTransaction types.Transaction    // The most recent revision to the contract.
	OriginConfirmed     bool                 // whether the origin transaction has been confirmed.
	RevisionConfirmed   bool                 // whether the most recent revision has been confirmed.
	ProofConfirmed      bool                 // whether the storage proof has been confirmed.

	// Where on disk the file is stored.
	Path string

	// The mutex ensures that revisions are happening in serial. The actual
	// data under the obligations is being protected by the host's mutex.
	// Grabbing 'mu' is not sufficient to guarantee modification safety of the
	// struct, the host mutex must also be grabbed.
	mu sync.Mutex
}

// isSane run several checks on a file contract obligation to make sure that
// all of the stateful assumptions hold. Listed below:
//  - There is exactly one file contract in OriginTransaction
//  - There is either one or zero file contracts in RevisionTransaction
//  - There are two 'validProofOutputs' and two 'missedProofOutputs' for each
//    file contract or revision.
//  - RevisionConfirmed is set to 'true' if there is no file contract revision.
//  - The ID is non-zero.
func (co *contractObligation) isSane() error {
	// Check that there is a file contract, and that it has the correct number
	// of valid and missed proof outputs.
	fclen := len(co.OriginTransaction.FileContracts)
	if fclen != 1 {
		return fmt.Errorf("obligation has bad file contract count: %v", fclen)
	}
	fcvpolen := len(co.OriginTransaction.FileContracts[0].ValidProofOutputs)
	if fcvpolen != 2 {
		return fmt.Errorf("obligation contract has bad valid proof output count: %v", fcvpolen)
	}
	fcmpolen := len(co.OriginTransaction.FileContracts[0].MissedProofOutputs)
	if fcmpolen != 2 {
		return fmt.Errorf("obligation contract has bad missed proof output count: %v", fcmpolen)
	}

	// Check that RevisionConfirmed is set to true if there is no revision.
	if !co.hasRevision() && !co.RevisionConfirmed {
		return errors.New("obligation has no revision, and no revision confirmation")
	}
	if !co.hasRevision() {
		// The rest of the function only pertains to obligations that contain
		// file contract revisions.
		return nil
	}

	// Check that there is exactly one revision, and that it has the correct
	// number number of valid and missed proof outputs.
	fcrlen := len(co.RevisionTransaction.FileContractRevisions)
	if fcrlen != 1 {
		return fmt.Errorf("obligation has bad revision count: %v", fcrlen)
	}
	fcrvpolen := len(co.RevisionTransaction.FileContractRevisions[0].NewValidProofOutputs)
	if fcrvpolen != 2 {
		return fmt.Errorf("obligation has bad revision valid proof output count: %v", fcrvpolen)
	}
	fcrmpolen := len(co.RevisionTransaction.FileContractRevisions[0].NewMissedProofOutputs)
	if fcrmpolen != 2 {
		return fmt.Errorf("obligation has bad revision missed proof output count: %v", fcrmpolen)
	}
	return nil
}

// fileSize returns the size of the file that is held by the contract
// obligation.
func (co *contractObligation) fileSize() uint64 {
	if co.hasRevision() {
		return co.RevisionTransaction.FileContractRevisions[0].NewFileSize
	}
	return co.OriginTransaction.FileContracts[0].FileSize
}

// hasRevision indiciates whether there is at least one file contract revision
// in the contract obligation.
func (co *contractObligation) hasRevision() bool {
	return len(co.RevisionTransaction.FileContractRevisions) > 0
}

// merkleRoot returns the operating unlock hash for a successful
// file contract in the obligation.
func (co *contractObligation) merkleRoot() crypto.Hash {
	if co.hasRevision() {
		return co.RevisionTransaction.FileContractRevisions[0].NewFileMerkleRoot
	}
	return co.OriginTransaction.FileContracts[0].FileMerkleRoot
}

// missedProofUnlockHash returns the operating unlock hash for a successful
// file contract in the obligation.
func (co *contractObligation) missedProofUnlockHash() types.UnlockHash {
	if co.hasRevision() {
		return co.RevisionTransaction.FileContractRevisions[0].NewMissedProofOutputs[1].UnlockHash
	}
	return co.OriginTransaction.FileContracts[0].MissedProofOutputs[1].UnlockHash
}

// payout returns the operating payout of the contract obligation.
func (co *contractObligation) payout() types.Currency {
	return co.OriginTransaction.FileContracts[0].Payout
}

// proofConfirmed inidicates whether the storage proofs have been seen on the
// blockchain.
func (co *contractObligation) proofConfirmed() bool {
	// Function seems unnecessary because it is just a getter, but adding this
	// function helps maintain consistency with the way that the other fields
	// are accessed.
	return co.ProofConfirmed
}

// reset updates the contract obligation to reflect that the consensus set is
// being rescanned, which means all of the consensus indicators need to be
// reset, and the action items need to be filled out again.
func (co *contractObligation) reset() {
	co.OriginConfirmed = false
	co.ProofConfirmed = false

	// If there is no revision, then the final revision counts as being
	// confirmed. Otherwise, the revision should not be considered as
	// confirmed.
	co.RevisionConfirmed = !co.hasRevision()
}

// revisionNumber returns the operating revision number of the obligation.
func (co *contractObligation) revisionNumber() uint64 {
	if co.hasRevision() {
		return co.RevisionTransaction.FileContractRevisions[0].NewRevisionNumber
	}
	return co.OriginTransaction.FileContracts[0].RevisionNumber
}

// txnsConfirmed indicates whether the file contract and its latest revision
// have been seen on the blockchain.
func (co *contractObligation) txnsConfirmed() bool {
	// The origin transaction is always present. If the origin transaction
	// isn't marked as confirmed, then the transactions aren't confirmed.
	if !co.OriginConfirmed {
		return false
	}

	// If there is a revision and it hasn't been confirmed, then the
	// transactions aren't confirmed.
	if co.hasRevision() && !co.RevisionConfirmed {
		return false
	}

	// Both potential fail conditions have been checked, all other possibilites
	// mean that all transactions have been confirmed.
	return true
}

// validProofUnlockHash returns the operating unlock hash for a successful file
// contract in the obligation.
func (co *contractObligation) validProofUnlockHash() types.UnlockHash {
	if co.hasRevision() {
		return co.RevisionTransaction.FileContractRevisions[0].NewValidProofOutputs[1].UnlockHash
	}
	return co.OriginTransaction.FileContracts[0].ValidProofOutputs[1].UnlockHash
}

// value returns the expected monetary value of the file contract.
func (co *contractObligation) value() types.Currency {
	if co.hasRevision() {
		return co.RevisionTransaction.FileContractRevisions[0].NewValidProofOutputs[1].Value
	}
	return co.OriginTransaction.FileContracts[0].ValidProofOutputs[1].Value
}

// unlockHash returns the operating unlock hash of the contract obligation.
func (co *contractObligation) unlockHash() types.UnlockHash {
	if co.hasRevision() {
		return co.RevisionTransaction.FileContractRevisions[0].NewUnlockHash
	}
	return co.OriginTransaction.FileContracts[0].UnlockHash
}

// windowStart returns the first block in the storage proof window of the
// contract obligation.
func (co *contractObligation) windowStart() types.BlockHeight {
	if co.hasRevision() {
		return co.RevisionTransaction.FileContractRevisions[0].NewWindowStart
	}
	return co.OriginTransaction.FileContracts[0].WindowStart
}

// windowEnd returns the first block in the storage proof window of the
// contract obligation.
func (co *contractObligation) windowEnd() types.BlockHeight {
	if co.hasRevision() {
		return co.RevisionTransaction.FileContractRevisions[0].NewWindowEnd
	}
	return co.OriginTransaction.FileContracts[0].WindowEnd
}

// addActionItem adds an action item at the given height for the given contract
// obligation.
func (h *Host) addActionItem(height types.BlockHeight, co *contractObligation) {
	_, exists := h.actionItems[height]
	if !exists {
		h.actionItems[height] = make(map[types.FileContractID]*contractObligation)
	}
	h.actionItems[height][co.ID] = co
}

// addObligation adds a new file contract obligation to the host. The
// obligation assumes that none of the transaction required by the obligation
// have not yet been confirmed on the blockchain.
func (h *Host) addObligation(co *contractObligation) {
	// 'addObligation' should not be adding an obligation that has a revision.
	if build.DEBUG && co.hasRevision() {
		panic("calling 'addObligation' with a file contract revision")
	}

	// Add the obligation to the list of host obligations.
	h.obligationsByID[co.ID] = co

	// The host needs to verify that the obligation transaction made it into
	// the blockchain.
	h.addActionItem(h.blockHeight+resubmissionTimeout, co)

	// Update the statistics.
	h.anticipatedRevenue = h.anticipatedRevenue.Add(co.value()) // Output at index 1 alone belongs to host.
	h.spaceRemaining = h.spaceRemaining - int64(co.fileSize())

	err := co.isSane()
	if err != nil {
		h.log.Critical("addObligation: obligation is not sane: " + err.Error())
	}
	err = h.save()
	if err != nil {
		h.log.Println("WARN: failed to save host:", err)
	}
}

// reviseObligation takes a file contract revision + transaction and applies it
// to an existing obligation.
func (h *Host) reviseObligation(revisionTransaction types.Transaction) {
	// Sanity checks - there should be exactly one revision in the transaction,
	// and that revision should correspond to a known obligation.
	fcrlen := len(revisionTransaction.FileContractRevisions)
	if fcrlen != 1 {
		h.log.Critical("reviseObligation: revisionTransaction has the wrong number of revisions:", fcrlen)
		return
	}
	obligation, exists := h.obligationsByID[revisionTransaction.FileContractRevisions[0].ParentID]
	if !exists {
		h.log.Critical("reviseObligation: revisionTransaction has no corresponding obligation")
		return
	}

	// Update the host's statistics.
	h.spaceRemaining += int64(obligation.fileSize())
	h.spaceRemaining -= int64(revisionTransaction.FileContractRevisions[0].NewFileSize)
	h.anticipatedRevenue = h.anticipatedRevenue.Sub(obligation.value())
	h.anticipatedRevenue = h.anticipatedRevenue.Add(revisionTransaction.FileContractRevisions[0].NewValidProofOutputs[1].Value)

	// The host needs to verify that the revision transaction made it into the
	// blockchain.
	h.addActionItem(h.blockHeight+resubmissionTimeout, obligation)

	// Add the revision to the obligation
	obligation.RevisionTransaction = revisionTransaction
	obligation.RevisionConfirmed = false

	err := obligation.isSane()
	if err != nil {
		h.log.Critical("reviseObligation: obligation is not sane: " + err.Error())
	}
	err = h.save()
	if err != nil {
		h.log.Println("WARN: failed to save host:", err)
	}
}

// removeObligation removes a file contract obligation and the corresponding
// file, allowing that space to be reallocated to new file contracts.
//
// TODO: The error handling in this function is not very tolerant.
func (h *Host) removeObligation(co *contractObligation, successful bool) {
	// Get the size of the file that's about to be removed.
	var size int64
	stat, err := os.Stat(co.Path)
	if err != nil {
		h.log.Println("ERROR: failed to remove obligation due to stat error:", err)
	} else {
		size = stat.Size()
	}

	// Remove the file and reallocate the space. If any of the operations fail,
	// none of the space will be re-added.
	err = os.Remove(co.Path)
	if err != nil {
		h.log.Println("ERROR: failed to remove obligation:", err)
	} else {
		h.spaceRemaining += size
	}

	// Update host statistics.
	h.anticipatedRevenue = h.anticipatedRevenue.Sub(co.value())
	if successful {
		h.revenue = h.revenue.Add(co.value())
	} else {
		h.lostRevenue = h.lostRevenue.Add(co.value())
	}

	// Remove the obligation from memory.
	delete(h.obligationsByID, co.ID)
	err = h.save()
	if err != nil {
		h.log.Println("ERROR: failed to save host:", err)
	}
}

// handleActionItem looks at a contract obligation and contextually determines
// if any actions need to be taken. Potential actions include submitting
// storage proofs, resubmitting file contracts, and deleting the obligation.
// handleActionItem will properly queue up any future actions that need to be
// taken.
func (h *Host) handleActionItem(co *contractObligation) {
	// Sanity check - after action is taken on the contract obligation, check
	// that the contract still satisfies the conditions for sanity.
	defer func() {
		err := co.isSane()
		if err != nil {
			h.log.Critical("handleActionItems: post-processing, obligation is insane: " + err.Error())
		}
	}()

	// Check that the obligation has not already been deleted.
	_, exists := h.obligationsByID[co.ID]
	if !exists {
		return
	}

	// Check for indications that the obligation should be deleted.
	if !co.txnsConfirmed() && co.windowStart() < h.blockHeight {
		// The final file contract revision has not made it into the
		// blockchain, and the window for submitting it has closed. The
		// obligation can be deleted.
		h.removeObligation(co, obligationFailed)
		return
	}
	if !co.proofConfirmed() && co.windowEnd() < h.blockHeight {
		// The storage proof has not been confirmed and the window for
		// submitting the proof has closed. The obligation can be deleted.
		h.removeObligation(co, obligationFailed)
		return
	}
	if co.proofConfirmed() && co.windowEnd()+confirmationRequirement <= h.blockHeight {
		// The storage proof has been successful, and has enough confirmations
		// to be considered stable. Therefore the obligation can be deleted.
		h.removeObligation(co, obligationSucceeded)
		return
	}

	// Check for actions that need to be performed immediately.
	if !co.OriginConfirmed {
		// The origin transaction has not been seen on the blockchain, and
		// should be resubmitted.
		err := h.tpool.AcceptTransactionSet([]types.Transaction{co.OriginTransaction})
		if err != nil {
			if _, ok := err.(modules.ConsensusConflict); ok {
				// The transaction has been rejected because it is in conflict
				// with the consensus database, usually meaning some type of
				// double spend has been confirmed on the blockchain. This
				// means that it's very unlikely that the file contract is
				// going to be accepted onto the blockchain at any point in the
				// future, and therefore the obligation should be removed.
				h.removeObligation(co, obligationFailed)
				h.log.Println("WARN: a file contract given to the host has been double spent!")
				return
			}
			h.log.Println("WARN: could not submit file contract transaction:", err)
		}
	}
	if !co.RevisionConfirmed && co.hasRevision() {
		// The revision transaction has not been seen on the blockchain, and
		// should be resubmitted.
		err := h.tpool.AcceptTransactionSet([]types.Transaction{co.RevisionTransaction})
		if err != nil {
			// There should be no circumstances under which the revision is
			// rejected from the transaction pool on the grounds of the
			// revision having been invalidated unless the original transaction
			// has also been removed. Therefore, it is not necessary to have
			// logic that determines whether the obligation should be deleted
			// due to the invalidity of the revision - all cases where the
			// obligation should be deleted will instead be handled by checking
			// the origin transaction.
			h.log.Println("WARN: could not submit file contract revision transaction:", err)
		}
	}
	if !co.txnsConfirmed() {
		// Not all of the transactions have been confirmed, which means
		// previous code in the function has submitted the transactions to the
		// blockchain. An action item should be added to check up on the
		// transaction status. Two blocks are waited to give the transactions
		// time to confirm in the event of network congestion.
		nextCheckup := h.blockHeight + resubmissionTimeout
		h.addActionItem(nextCheckup, co)
		return
	}

	// If this part of the code is being reached, everything has succeeded
	// except for maybe the storage proof. The remaining code is for handling
	// the storage proof.
	if co.windowStart() > h.blockHeight {
		// The file contract has not expired, which means it is too early to
		// create or submit a storage proof. Set an action item in the future
		// to handle the storage proof.
		nextCheckup := co.windowStart() + resubmissionTimeout
		h.addActionItem(nextCheckup, co)
		return
	}
	if !co.proofConfirmed() && co.windowStart() <= h.blockHeight {
		// The storage proof for the contract has not made it onto the
		// blockchain, recreate the storage proof and submit it to the
		// blockchain.
		go h.threadedCreateStorageProof(co)

		// Add an action to check that the storage proof has been successful.
		nextCheckup := h.blockHeight + resubmissionTimeout
		h.addActionItem(nextCheckup, co)
		return
	}
	if co.proofConfirmed() {
		// The storage proof has been confirmed, but the obligation is held
		// until there have been enough confirmations on the storage proof to
		// be certain that a different storage proof won't need to be created.
		// Add an action item that will trigger once the storage proof has
		// enough confirmations.
		nextCheckup := co.windowEnd() + confirmationRequirement
		h.addActionItem(nextCheckup, co)
		return
	}

	// All possible scenarios should be covered. This code should not be
	// reachable.
	h.log.Critical("logic error - bottom of handleActionItems has been reached")
}
