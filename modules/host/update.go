package host

import (
	"fmt"
	"os"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// threadedDeleteObligation deletes a file obligation.
func (h *Host) threadedDeleteObligation(obligation contractObligation) {
	lockID := h.mu.Lock()
	defer h.mu.Unlock(lockID)
	err := h.deallocate(obligation.Path)
	if err != nil {
		fmt.Println(err)
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
		fmt.Println(err)
		return
	}
	defer file.Close()

	segmentIndex, err := h.cs.StorageProofSegment(obligation.ID)
	if err != nil {
		fmt.Println(err)
		return
	}
	base, hashSet, err := crypto.BuildReaderProof(file, segmentIndex)
	if err != nil {
		fmt.Println(err)
		return
	}
	sp := types.StorageProof{obligation.ID, [crypto.SegmentSize]byte{}, hashSet}
	copy(sp.Segment[:], base)

	// Create and send the transaction.
	txnBuilder := h.wallet.StartTransaction()
	txnBuilder.AddStorageProof(sp)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = h.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Storage proof was successful, so increment profit tracking
	lockID := h.mu.Lock()
	h.profit = h.profit.Add(obligation.FileContract.Payout)
	h.mu.Unlock(lockID)
}

// ProcessConsensusChange will be called by the consensus set every time there
// is a change to the blockchain.
func (h *Host) ProcessConsensusChange(cc modules.ConsensusChange) {
	lockID := h.mu.Lock()
	defer h.mu.Unlock(lockID)

	h.blockHeight -= types.BlockHeight(len(cc.RevertedBlocks))

	// Check the applied blocks and see if any of the contracts we have are
	// ready for storage proofs.
	for _ = range cc.AppliedBlocks {
		h.blockHeight++
		for _, obligation := range h.obligationsByHeight[h.blockHeight] {
			go h.threadedCreateStorageProof(obligation)
		}
		// TODO: If something happens while the storage proofs are being
		// created, those files will never get cleared from the host.
		delete(h.obligationsByHeight, h.blockHeight)
	}
	h.consensusHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	h.consensusHeight += types.BlockHeight(len(cc.AppliedBlocks))
}
