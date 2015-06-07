package host

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Create a proof of storage for a contract, using the state height to
// determine the random seed. Create proof must be under a host and state lock.
func (h *Host) createStorageProof(obligation contractObligation, heightForProof types.BlockHeight) (err error) {
	fullpath := filepath.Join(h.saveDir, obligation.Path)
	file, err := os.Open(fullpath)
	if err != nil {
		return
	}
	defer file.Close()

	segmentIndex, err := h.cs.StorageProofSegment(obligation.ID)
	if err != nil {
		return
	}
	base, hashSet, err := crypto.BuildReaderProof(file, segmentIndex)
	if err != nil {
		return
	}

	sp := types.StorageProof{obligation.ID, base, hashSet}

	// Create and send the transaction.
	id, err := h.wallet.RegisterTransaction(types.Transaction{})
	if err != nil {
		return
	}
	_, _, err = h.wallet.AddStorageProof(id, sp)
	if err != nil {
		return
	}
	t, err := h.wallet.SignTransaction(id, true)
	if err != nil {
		return
	}
	err = h.tpool.AcceptTransaction(t)
	if err != nil {
		if build.DEBUG {
			panic(err)
		}
	}

	return
}

// RecieveConsensusSetUpdate will be called by the consensus set every time
// there is a new block or a fork of some kind.
func (h *Host) ReceiveConsensusSetUpdate(cc modules.ConsensusChange) {
	lockID := h.mu.Lock()
	defer h.mu.Unlock(lockID)

	h.blockHeight -= types.BlockHeight(len(cc.RevertedBlocks))

	// Check the applied blocks and see if any of the contracts we have are
	// ready for storage proofs.
	shouldSave := false
	for _ = range cc.AppliedBlocks {
		h.blockHeight++

		for _, obligation := range h.obligationsByHeight[h.blockHeight] {
			// Submit a storage proof for the obligation.
			err := h.createStorageProof(obligation, h.blockHeight)
			if err != nil {
				fmt.Println(err)
				return
			}

			// Storage proof was successful, so increment profit tracking
			h.profit = h.profit.Add(obligation.FileContract.Payout)

			// Delete the obligation.
			fullpath := filepath.Join(h.saveDir, obligation.Path)
			stat, err := os.Stat(fullpath)
			if err != nil {
				fmt.Println(err)
				return
			}
			h.deallocate(uint64(stat.Size()), obligation.Path) // TODO: file might actually be the wrong size.
			shouldSave = true
			delete(h.obligationsByID, obligation.ID)
		}
		delete(h.obligationsByHeight, h.blockHeight)
	}
	if shouldSave {
		_ = h.save() // TODO: Some way to communicate that the save failed.
	}

	go h.updateSubscribers()
}
