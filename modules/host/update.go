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

		if block.ID() != types.GenesisBlock.ID() {
			h.blockHeight--
		}
	}
	for _, block := range cc.AppliedBlocks {
		for _, txn := range block.Transactions {
			// Look for file contracts, revisions, and storage proofs that have
			// been confirmed on-chain.
			for k := range txn.FileContracts {
				ob, exists := h.obligationsByID[txn.FileContractID(uint64(k))]
				if exists {
					ob.OriginConfirmed = true

					// COMPATv0.4.8 - found the original transaction on the
					// chain, can copy it over for compatibility. An equivalent
					// check is not needed when rewinding, as a rescan is
					// performed that puts the host knowledge at the tip of
					// consensus.
					ob.OriginTransaction = txn
				}
			}
			for _, fcr := range txn.FileContractRevisions {
				ob, exists := h.obligationsByID[fcr.ParentID]
				if exists && ob.revisionNumber() == fcr.NewRevisionNumber {
					// COMPATv0.4.8 - found a revision, can move it over to get
					// compatibility. No harm is done by adding the revision as
					// long as it is set to 'confirmed' after the fact.
					h.reviseObligation(txn)

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
		if block.ID() != types.GenesisBlock.ID() {
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
