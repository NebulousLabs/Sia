package host

import (
	"os"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// threadedDeleteObligation deletes a file obligation.
func (h *Host) threadedDeleteObligation(obligation *contractObligation) {
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
func (h *Host) threadedCreateStorageProof(obligation *contractObligation) {
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

	// Adjust the height of the host. The host height is initialized to zero,
	// but the genesis block is actually height zero. For the genesis block
	// only, the height will be left at zero.
	//
	// Checking the height here eliminates the need to initialize the host to
	// and underflowed types.BlockHeight.
	oldHeight := h.blockHeight
	if h.blockHeight != 0 || cc.AppliedBlocks[len(cc.AppliedBlocks)-1].ID() != h.cs.GenesisBlock().ID() {
		h.blockHeight -= types.BlockHeight(len(cc.RevertedBlocks))
		h.blockHeight += types.BlockHeight(len(cc.AppliedBlocks))
	}

	// Check the range of heights between the previous height and the current
	// height for storage proof obligations. There is no mechanism for
	// re-submitting a storage proof in the event of a deep reorg, but the host
	// waits StorageProofReorgDepth confirmations before submitting a storage
	// proof. Reorgs deeper than that are assumed to be rare enough that it's
	// okay for the host to eat losses under those circumstances.
	for oldHeight < h.blockHeight {
		oldHeight++
		for _, ob := range h.obligationsByHeight[oldHeight] {
			go h.threadedCreateStorageProof(ob)
		}
		delete(h.obligationsByHeight, oldHeight)
	}
}
