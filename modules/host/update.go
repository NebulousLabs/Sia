package host

import (
	"os"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// initRescan is a helper funtion of initConsensusSubscribe, and is called when
// the host and the consensus set have become desynchronized. Desynchronization
// typically happens if the user is replacing or altering the persistent files
// in the consensus set or the host.
func (h *Host) initRescan() error {
	// Reset all of the variables that have relevance to the consensus set.
	err := func() error {
		h.mu.Lock()
		defer h.mu.Unlock()

		// Reset all of the consensus-relevant variables in the host.
		h.blockHeight = 0
		h.actionItems = make(map[types.BlockHeight]map[types.FileContractID]*contractObligation)
		for _, ob := range h.obligationsByID {
			ob.reset()
		}

		return h.save()
	}()
	if err != nil {
		return err
	}

	// Subscribe to the consensus set. This is a blocking call that will not
	// return until the host has fully caught up to the current block.
	err = h.cs.ConsensusSetPersistentSubscribe(h, modules.ConsensusChangeID{})
	if err != nil {
		return err
	}

	// After the host has caught up to the current consensus, perform any
	// necessary actions on each obligation, including adding necessary future
	// actions (such as creating storage proofs) to the queue.
	for _, ob := range h.obligationsByID {
		h.handleActionItem(ob)
	}
	return nil
}

// initConsensusSubscription subscribes the host to the consensus set.
func (h *Host) initConsensusSubscription() error {
	err := h.cs.ConsensusSetPersistentSubscribe(h, h.recentChange)
	if err == modules.ErrInvalidConsensusChangeID {
		// Perform a rescan of the consensus set if the change id that the host
		// has is unrecognized by the consensus set. This will typically only
		// happen if the user has been replacing files inside the folder
		// structure.
		return h.initRescan()
	}
	return err
}

// threadedCreateStorageProof creates a storage proof for a file contract
// obligation and submits it to the blockchain. Though a lock is never held, a
// significant amount of disk I/O happens, meaning this function should be
// called in a separate goroutine.
func (h *Host) threadedCreateStorageProof(obligation *contractObligation) {
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if build.DEBUG && h.closed {
		panic("the close order should guarantee that threadedCreateStorageProof has access to host resources - yet host is closed!")
	}

	file, err := os.Open(obligation.Path)
	if err != nil {
		h.log.Printf("ERROR: could not open obligation %v (%v) for storage proof: %v", obligation.ID, obligation.Path, err)
		return
	}
	defer file.Close()

	segmentIndex, err := h.cs.StorageProofSegment(obligation.ID)
	if err != nil {
		h.log.Printf("ERROR: could not determine storage proof index for %v (%v): %v", obligation.ID, obligation.Path, err)
		return
	}
	base, hashSet, err := crypto.BuildReaderProof(file, segmentIndex)
	if err != nil {
		h.log.Printf("ERROR: could not construct storage proof for %v (%v): %v", obligation.ID, obligation.Path, err)
		return
	}
	sp := types.StorageProof{
		ParentID: obligation.ID,
		HashSet:  hashSet,
	}
	copy(sp.Segment[:], base)

	// Create and send the transaction.
	txnBuilder := h.wallet.StartTransaction()
	txnBuilder.AddStorageProof(sp)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		h.log.Println("couldn't sign storage proof transaction:", err)
		return
	}
	err = h.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		h.log.Printf("ERROR: could not submit storage proof txn for %v (%v): %v", obligation.ID, obligation.Path, err)
		return
	}
}

// handleActionItem looks at a contract obligation and contextually determines
// if any actions need to be taken. Potential actions include submitting
// storage proofs, resubmitting file contracts, and deleting the obligation.
// handleActionItem will properly queue up any future actions that need to be
// taken.
func (h *Host) handleActionItem(co *contractObligation) {
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
		err := h.tpool.AcceptTransactionSet([]types.Transaction{co.OriginTxn})
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
		err := h.tpool.AcceptTransactionSet([]types.Transaction{co.RevisionTxn})
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
		nextCheckup := h.blockHeight + 2
		h.addActionItem(nextCheckup, co)
		return
	}

	// If this part of the code is being reached, everything has succeeded
	// except for maybe the storage proof. The remaining code is for handling
	// the storage proof.
	if co.windowStart() >= h.blockHeight {
		// The file contract has not expired, which means it is too early to
		// create or submit a storage proof. Set an action item in the future
		// to handle the storage proof.
		nextCheckup := co.windowStart() + 1
		h.addActionItem(nextCheckup, co)
		return
	}
	if !co.proofConfirmed() && co.windowStart() < h.blockHeight {
		// The storage proof for the contract has not made it onto the
		// blockchain, recreate the storage proof and submit it to the
		// blockchain.
		go h.threadedCreateStorageProof(co)

		// Add an action to check that the storage proof has been successful.
		nextCheckup := h.blockHeight + 2
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

	// At this point, all possible scenarios should be covered. This part of
	// the function should not be reachable.
	if build.DEBUG {
		panic("logic error - unreachable code has been hit")
	}
}

// ProcessConsensusChange will be called by the consensus set every time there
// is a change to the blockchain.
func (h *Host) ProcessConsensusChange(cc modules.ConsensusChange) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Go through the diffs and track any obligation-relevant transactions that
	// have been confirmed or reorged.
	for _, block := range cc.RevertedBlocks {
		for _, txn := range block.Transactions {
			// Look for file contracts, revisions, and storage proofs that are
			// no longer confirmed on-chain.
			for k := range txn.FileContracts {
				ob, exists := h.obligationsByID[txn.FileContractID(uint64(k))]
				if exists {
					ob.OriginConfirmed = false
				}
			}
			for _, fcr := range txn.FileContractRevisions {
				ob, exists := h.obligationsByID[fcr.ParentID]
				if exists {
					// There is a guarantee in the blockchain that if a
					// revision for this file contract id is being removed, the
					// most recent revision on the file contract is not in the
					// blockchain. This guarantee only holds so long as the
					// host has the revision with the highest revision number,
					// and as long as there is only one revision signed with
					// each revision number.
					ob.RevisionConfirmed = false
				}
			}
			for _, sp := range txn.StorageProofs {
				ob, exists := h.obligationsByID[sp.ParentID]
				if exists {
					ob.ProofConfirmed = false
				}
			}
		}
		h.blockHeight--
	}
	for _, block := range cc.AppliedBlocks {
		for _, txn := range block.Transactions {
			// Look for file contracts, revisions, and storage proofs that have
			// been confirmed on-chain.
			for k := range txn.FileContracts {
				ob, exists := h.obligationsByID[txn.FileContractID(uint64(k))]
				if exists {
					ob.OriginConfirmed = true
				}
			}
			for _, fcr := range txn.FileContractRevisions {
				ob, exists := h.obligationsByID[fcr.ParentID]
				if exists && ob.revisionNumber() == fcr.NewRevisionNumber {
					// Need to check that the revision is the most recent
					// revision. By assuming that the host only signs one
					// revision for each number, and that the host has the most
					// recent revision in the obligation, just checking the
					// revision number is sufficient.
					ob.RevisionConfirmed = true
				}
			}
			for _, sp := range txn.StorageProofs {
				ob, exists := h.obligationsByID[sp.ParentID]
				if exists {
					ob.ProofConfirmed = true
				}
			}
		}

		// Adjust the height of the host. The host height is initialized to
		// zero, but the genesis block is actually height zero. For the genesis
		// block only, the height will be left at zero.
		//
		// Checking the height here eliminates the need to initialize the host
		// to and underflowed types.BlockHeight.
		if h.blockHeight != 0 || cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID() != h.cs.GenesisBlock().ID() {
			h.blockHeight++
		}

		// Handle any action items that have been scheduled for the current
		// height.
		obMap := h.actionItems[h.blockHeight]
		for _, ob := range obMap {
			h.handleActionItem(ob)
		}
		delete(h.actionItems, h.blockHeight)
	}

	// Update the host's recent change pointer to point to the most recent
	// change.
	h.recentChange = cc.ID

	// Save the host.
	err := h.save()
	if err != nil {
		h.log.Println("ERROR: could not save during ProcessConsensusChange:", err)
	}
}
