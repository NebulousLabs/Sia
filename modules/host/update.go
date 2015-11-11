package host

import (
	"os"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// threadedDeleteObligation deletes a file obligation.
func (h *Host) threadedDeleteObligation(obligation contractObligation) {
	h.mu.Lock()
	defer h.mu.Unlock()
	err := h.deallocate(obligation.Path)
	if err != nil {
		h.log.Printf("WARN: failed to deallocate %v: %v", obligation.Path, err)
	}
	delete(h.obligationsByID, obligation.ID)
	h.save()
}

// threadedCreateStorageProof creates a storage proof for a file contract
// obligation and submits it to the blockchain.
//
// TODO: The printlns here should be logging messages.
func (h *Host) threadedCreateStorageProof(obligation contractObligation) {
	defer h.threadedDeleteObligation(obligation)

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
	sp := types.StorageProof{obligation.ID, [crypto.SegmentSize]byte{}, hashSet}
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

	// Storage proof was successful, so increment profit tracking
	h.mu.Lock()
	h.profit = h.profit.Add(obligation.FileContract.Payout)
	h.mu.Unlock()
}

// ProcessConsensusChange will be called by the consensus set every time there
// is a change to the blockchain.
func (h *Host) ProcessConsensusChange(cc modules.ConsensusChange) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.blockHeight += types.BlockHeight(len(cc.AppliedBlocks))
	h.blockHeight -= types.BlockHeight(len(cc.RevertedBlocks))

	// Check the applied blocks and see if any of the contracts we have are
	// ready for storage proofs.
	for _ = range cc.AppliedBlocks {
		for _, ob := range h.obligationsByHeight[h.blockHeight] {
			// to avoid race conditions involving the obligation's mutex, copy it
			// manually into a new object
			obcopy := contractObligation{ID: ob.ID, FileContract: ob.FileContract, LastRevisionTxn: ob.LastRevisionTxn}
			go h.threadedCreateStorageProof(obcopy)
		}
		// TODO: If something happens while the storage proofs are being
		// created, those files will never get cleared from the host.
		delete(h.obligationsByHeight, h.blockHeight)
	}
}
