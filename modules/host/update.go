package host

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
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

	segmentIndex, err := h.state.StorageProofSegment(obligation.ID)
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

// update grabs all of the blocks that have appeared since the last update and
// submits any necessary storage proofs.
func (h *Host) update() {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Get the blocks since the recent update.
	_, appliedBlockIDs, err := h.state.BlocksSince(h.latestBlock)
	if err != nil {
		// The host has somehow desynchronized.
		if build.DEBUG {
			panic(err)
		}
	}
	if len(appliedBlockIDs) == 0 {
		return
	}
	h.latestBlock = appliedBlockIDs[len(appliedBlockIDs)-1]

	// Check the applied blocks and see if any of the contracts we have are
	// ready for storage proofs.
	for _, blockID := range appliedBlockIDs {
		height, exists := h.state.HeightOfBlock(blockID)
		if build.DEBUG {
			if !exists {
				panic("a block returned by BlocksSince doesn't appear to exist")
			}
		}

		for _, obligation := range h.obligationsByHeight[height] {
			// Submit a storage proof for the obligation.
			err := h.createStorageProof(obligation, h.state.Height())
			if err != nil {
				fmt.Println(err)
				return
			}

			// Delete the obligation.
			fullpath := filepath.Join(h.saveDir, obligation.Path)
			stat, err := os.Stat(fullpath)
			if err != nil {
				fmt.Println(err)
				return
			}
			h.deallocate(uint64(stat.Size()), obligation.Path) // TODO: file might actually be the wrong size.

			delete(h.obligationsByID, obligation.ID)
		}
		delete(h.obligationsByHeight, height)
	}
}

// threadedConsensusListen listens to a channel that's subscribed to the state
// and updates every time the consensus set changes. When the consensus set
// changes, the host checks if there are any storage proofs that need to be
// submitted and submits them.
func (h *Host) threadedConsensusListen(consensusChan <-chan struct{}) {
	for _ = range consensusChan {
		h.update()
	}
}
