package host

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// threadedDeleteObligation deletes a file obligation.
func (h *Host) threadedDeleteObligation(obligation contractObligation) {
	// Delete the obligation.
	lockID := h.mu.Lock()
	defer h.mu.Unlock(lockID)

	fullpath := filepath.Join(h.saveDir, obligation.Path)
	stat, err := os.Stat(fullpath)
	if err != nil {
		fmt.Println(err)
	}
	h.deallocate(uint64(stat.Size()), obligation.Path)
	delete(h.obligationsByID, obligation.ID)

	// Storage proof was successful, so increment profit tracking
	h.profit = h.profit.Add(obligation.FileContract.Payout)

	_ = h.save() // TODO: Some way to communicate that the save failed.
}

// threadedCreateStorageProof creates a storage proof for a file contract
// obligation and submits it to the blockchain.
func (h *Host) threadedCreateStorageProof(obligation contractObligation, heightForProof types.BlockHeight) {
	defer h.threadedDeleteObligation(obligation)

	fullpath := filepath.Join(h.saveDir, obligation.Path)
	file, err := os.Open(fullpath)
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
	id, err := h.wallet.RegisterTransaction(types.Transaction{})
	if err != nil {
		fmt.Println(err)
		return
	}
	_, _, err = h.wallet.AddStorageProof(id, sp)
	if err != nil {
		fmt.Println(err)
		return
	}
	t, err := h.wallet.SignTransaction(id, true)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = h.tpool.AcceptTransactionSet(t)
	if err != nil {
		fmt.Println(err)
		return
	}
}

// RecieveConsensusSetUpdate will be called by the consensus set every time
// there is a new block or a fork of some kind.
func (h *Host) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	lockID := h.mu.Lock()
	defer h.mu.Unlock(lockID)

	h.blockHeight -= types.BlockHeight(len(cc.RevertedBlocks))

	// Check the applied blocks and see if any of the contracts we have are
	// ready for storage proofs.
	for _ = range cc.AppliedBlocks {
		h.blockHeight++
		for _, obligation := range h.obligationsByHeight[h.blockHeight] {
			go h.threadedCreateStorageProof(obligation, h.blockHeight)
		}
		// TODO: If something happens while the storage proofs are being
		// created, those files will never get cleared from the host.
		delete(h.obligationsByHeight, h.blockHeight)
	}
	h.consensusHeight -= types.BlockHeight(len(cc.RevertedBlocks))
	h.consensusHeight += types.BlockHeight(len(cc.AppliedBlocks))
}
