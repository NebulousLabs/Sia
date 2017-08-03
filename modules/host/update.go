package host

// TODO: Need to check that 'RevisionConfirmed' is sensitive to whether or not
// it was the *most recent* revision that got confirmed.

import (
	"encoding/binary"
	"encoding/json"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// initRescan is a helper function of initConsensusSubscribe, and is called when
// the host and the consensus set have become desynchronized. Desynchronization
// typically happens if the user is replacing or altering the persistent files
// in the consensus set or the host.
func (h *Host) initRescan() error {
	// Reset all of the variables that have relevance to the consensus set.
	var allObligations []storageObligation
	// Reset all of the consensus-relevant variables in the host.
	h.blockHeight = 0

	// Reset all of the storage obligations.
	err := h.db.Update(func(tx *bolt.Tx) error {
		bsu := tx.Bucket(bucketStorageObligations)
		c := bsu.Cursor()
		for k, soBytes := c.First(); soBytes != nil; k, soBytes = c.Next() {
			var so storageObligation
			err := json.Unmarshal(soBytes, &so)
			if err != nil {
				return err
			}
			so.OriginConfirmed = false
			so.RevisionConfirmed = false
			so.ProofConfirmed = false
			allObligations = append(allObligations, so)
			soBytes, err = json.Marshal(so)
			if err != nil {
				return err
			}
			err = bsu.Put(k, soBytes)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Subscribe to the consensus set. This is a blocking call that will not
	// return until the host has fully caught up to the current block.
	//
	// Convention dictates that the host should not make external calls while
	// under lock, but this function happens at startup while blocking. Because
	// it happens while blocking, and because there is no actual host lock held
	// at this time, none of the host external functions are exposed, so it is
	// save to make the exported call.
	err = h.cs.ConsensusSetSubscribe(h, modules.ConsensusChangeBeginning, h.tg.StopChan())
	if err != nil {
		return err
	}
	h.tg.OnStop(func() {
		h.cs.Unsubscribe(h)
	})

	// Re-queue all of the action items for the storage obligations.
	for i, so := range allObligations {
		soid := so.id()
		err1 := h.queueActionItem(h.blockHeight+resubmissionTimeout, soid)
		err2 := h.queueActionItem(so.expiration()-revisionSubmissionBuffer, soid)
		err3 := h.queueActionItem(so.expiration()+resubmissionTimeout, soid)
		err = composeErrors(err1, err2, err3)
		if err != nil {
			h.log.Println("dropping storage obligation during rescan, id", so.id())
		}

		// AcceptTransactionSet needs to be called in a goroutine to avoid a
		// deadlock.
		go func(i int) {
			err := h.tpool.AcceptTransactionSet(allObligations[i].OriginTransactionSet)
			if err != nil {
				h.log.Println("Unable to submit contract transaction set after rescan:", soid)
			}
		}(i)
	}
	return nil
}

// initConsensusSubscription subscribes the host to the consensus set.
func (h *Host) initConsensusSubscription() error {
	// Convention dictates that the host should not make external calls while
	// under lock, but this function happens at startup while blocking. Because
	// it happens while blocking, and because there is no actual host lock held
	// at this time, none of the host external functions are exposed, so it is
	// save to make the exported call.
	err := h.cs.ConsensusSetSubscribe(h, h.recentChange, h.tg.StopChan())
	if err == modules.ErrInvalidConsensusChangeID {
		// Perform a rescan of the consensus set if the change id that the host
		// has is unrecognized by the consensus set. This will typically only
		// happen if the user has been replacing files inside the Sia folder
		// structure.
		return h.initRescan()
	}
	if err != nil {
		return err
	}
	h.tg.OnStop(func() {
		h.cs.Unsubscribe(h)
	})
	return nil
}

// ProcessConsensusChange will be called by the consensus set every time there
// is a change to the blockchain.
func (h *Host) ProcessConsensusChange(cc modules.ConsensusChange) {
	// Add is called at the beginning of the function, but Done cannot be
	// called until all of the threads spawned by this function have also
	// terminated. This function should not block while these threads wait to
	// terminate.
	h.mu.Lock()
	defer h.mu.Unlock()

	// Wrap the whole parsing into a single large database tx to keep things
	// efficient.
	var actionItems []types.FileContractID
	err := h.db.Update(func(tx *bolt.Tx) error {
		for _, block := range cc.RevertedBlocks {
			// Look for transactions relevant to open storage obligations.
			for _, txn := range block.Transactions {
				// Check for file contracts.
				if len(txn.FileContracts) > 0 {
					for j := range txn.FileContracts {
						fcid := txn.FileContractID(uint64(j))
						so, err := getStorageObligation(tx, fcid)
						if err != nil {
							// The storage folder may not exist, or the disk
							// may be having trouble. Either way, we ignore the
							// problem. If the disk is having trouble, the user
							// will have to perform a rescan.
							continue
						}
						so.OriginConfirmed = false
						err = putStorageObligation(tx, so)
						if err != nil {
							continue
						}
					}
				}

				// Check for file contract revisions.
				if len(txn.FileContractRevisions) > 0 {
					for _, fcr := range txn.FileContractRevisions {
						so, err := getStorageObligation(tx, fcr.ParentID)
						if err != nil {
							// The storage folder may not exist, or the disk
							// may be having trouble. Either way, we ignore the
							// problem. If the disk is having trouble, the user
							// will have to perform a rescan.
							continue
						}
						so.RevisionConfirmed = false
						err = putStorageObligation(tx, so)
						if err != nil {
							continue
						}
					}
				}

				// Check for storage proofs.
				if len(txn.StorageProofs) > 0 {
					for _, sp := range txn.StorageProofs {
						// Check database for relevant storage proofs.
						so, err := getStorageObligation(tx, sp.ParentID)
						if err != nil {
							// The storage folder may not exist, or the disk
							// may be having trouble. Either way, we ignore the
							// problem. If the disk is having trouble, the user
							// will have to perform a rescan.
							continue
						}
						so.ProofConfirmed = false
						err = putStorageObligation(tx, so)
						if err != nil {
							continue
						}
					}
				}
			}

			// Height is not adjusted when dealing with the genesis block because
			// the default height is 0 and the genesis block height is 0. If
			// removing the genesis block, height will already be at height 0 and
			// should not update, lest an underflow occur.
			if block.ID() != types.GenesisID {
				h.blockHeight--
			}
		}
		for _, block := range cc.AppliedBlocks {
			// Look for transactions relevant to open storage obligations.
			for _, txn := range block.Transactions {
				// Check for file contracts.
				if len(txn.FileContracts) > 0 {
					for i := range txn.FileContracts {
						fcid := txn.FileContractID(uint64(i))
						so, err := getStorageObligation(tx, fcid)
						if err != nil {
							// The storage folder may not exist, or the disk
							// may be having trouble. Either way, we ignore the
							// problem. If the disk is having trouble, the user
							// will have to perform a rescan.
							continue
						}
						so.OriginConfirmed = true
						err = putStorageObligation(tx, so)
						if err != nil {
							continue
						}
					}
				}

				// Check for file contract revisions.
				if len(txn.FileContractRevisions) > 0 {
					for _, fcr := range txn.FileContractRevisions {
						so, err := getStorageObligation(tx, fcr.ParentID)
						if err != nil {
							// The storage folder may not exist, or the disk
							// may be having trouble. Either way, we ignore the
							// problem. If the disk is having trouble, the user
							// will have to perform a rescan.
							continue
						}
						so.RevisionConfirmed = true
						err = putStorageObligation(tx, so)
						if err != nil {
							continue
						}
					}
				}

				// Check for storage proofs.
				if len(txn.StorageProofs) > 0 {
					for _, sp := range txn.StorageProofs {
						so, err := getStorageObligation(tx, sp.ParentID)
						if err != nil {
							// The storage folder may not exist, or the disk
							// may be having trouble. Either way, we ignore the
							// problem. If the disk is having trouble, the user
							// will have to perform a rescan.
							continue
						}
						so.ProofConfirmed = true
						err = putStorageObligation(tx, so)
						if err != nil {
							continue
						}
					}
				}
			}

			// Height is not adjusted when dealing with the genesis block because
			// the default height is 0 and the genesis block height is 0. If adding
			// the genesis block, height will already be at height 0 and should not
			// update.
			if block.ID() != types.GenesisID {
				h.blockHeight++
			}

			// Handle any action items relevant to the current height.
			bai := tx.Bucket(bucketActionItems)
			heightBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(heightBytes, uint64(h.blockHeight)) // BigEndian used so bolt will keep things sorted automatically.
			existingItems := bai.Get(heightBytes)

			// From the existing items, pull out a storage obligation.
			knownActionItems := make(map[types.FileContractID]struct{})
			obligationIDs := make([]types.FileContractID, len(existingItems)/crypto.HashSize)
			for i := 0; i < len(existingItems); i += crypto.HashSize {
				copy(obligationIDs[i/crypto.HashSize][:], existingItems[i:i+crypto.HashSize])
			}
			for _, soid := range obligationIDs {
				_, exists := knownActionItems[soid]
				if !exists {
					actionItems = append(actionItems, soid)
					knownActionItems[soid] = struct{}{}
				}
			}
		}
		return nil
	})
	if err != nil {
		h.log.Println(err)
	}
	for i := range actionItems {
		go h.threadedHandleActionItem(actionItems[i])
	}

	// Update the host's recent change pointer to point to the most recent
	// change.
	h.recentChange = cc.ID

	// Save the host.
	err = h.saveSync()
	if err != nil {
		h.log.Println("ERROR: could not save during ProcessConsensusChange:", err)
	}
}
