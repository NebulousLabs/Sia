package host

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/crypto"
)

// Create a proof of storage for a contract, using the state height to
// determine the random seed. Create proof must be under a host and state lock.
func (h *Host) createStorageProof(obligation contractObligation, heightForProof consensus.BlockHeight) (err error) {
	file, err := os.Open(obligation.path)
	if err != nil {
		return
	}
	defer file.Close()

	segmentIndex, err := h.state.StorageProofSegment(obligation.id)
	if err != nil {
		return
	}
	numSegments := crypto.CalculateSegments(obligation.fileContract.FileSize)
	base, hashSet, err := crypto.BuildReaderProof(file, numSegments, segmentIndex)
	if err != nil {
		return
	}
	sp := consensus.StorageProof{obligation.id, base, hashSet}

	// Create and send the transaction.
	id, err := h.wallet.RegisterTransaction(consensus.Transaction{})
	if err != nil {
		return
	}
	err = h.wallet.AddStorageProof(id, sp)
	if err != nil {
		return
	}
	t, err := h.wallet.SignTransaction(id, true)
	if err != nil {
		return
	}
	h.tpool.AcceptTransaction(t)

	return
}

// threadedConsensusListen listens to a channel that's subscribed to the state
// and updates every time the consensus set changes. When the consensus set
// changes, the host checks if there are any storage proofs that need to be
// submitted and submits them.
func (h *Host) threadedConsensusListen(consensusChan chan struct{}) {
	for _ = range consensusChan {
		h.mu.Lock()

		// Get the blocks since the recent update.
		_, appliedBlockIDs, err := h.state.BlocksSince(h.latestBlock)
		if err != nil {
			// The host has somehow desynchronized.
			if consensus.DEBUG {
				panic(err)
			}
		}
		h.latestBlock = h.state.CurrentBlock().ID()

		// Check the applied blocks and see if any of the contracts we have are
		// ready for storage proofs.
		for _, blockID := range appliedBlockIDs {
			height, exists := h.state.HeightOfBlock(blockID)
			if consensus.DEBUG {
				if !exists {
					panic("a block returned by BlocksSince doesn't appear to exist")
				}
			}

			for _, obligation := range h.obligationsByHeight[height] {
				// Submit a storage proof for the obligation.
				err := h.createStorageProof(obligation, h.state.Height())
				if err != nil {
					fmt.Println(err)
					continue
				}

				// Delete the obligation.
				fullpath := filepath.Join(h.hostDir, obligation.path)
				stat, err := os.Stat(fullpath)
				if err != nil {
					fmt.Println(err)
				}
				err = os.Remove(fullpath)
				h.spaceRemaining += stat.Size()
				if err != nil {
					fmt.Println(err)
				}

				delete(h.obligationsByID, obligation.id)
			}
			delete(h.obligationsByHeight, height)
		}

		h.mu.Unlock()
	}
}
