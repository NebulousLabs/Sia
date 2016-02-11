package host

import (
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

		// TODO: Need to reset all of the storage obligations.

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

	// TODO: Need to re-queue all of the action items for the storage
	// obligations.

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

// ProcessConsensusChange will be called by the consensus set every time there
// is a change to the blockchain.
func (h *Host) ProcessConsensusChange(cc modules.ConsensusChange) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, block := range cc.RevertedBlocks {
		// TODO: Look for transactions relevant to open storage obligations.

		if block.ID() != types.GenesisBlock.ID() {
			h.blockHeight--
		}
	}
	for _, block := range cc.AppliedBlocks {
		// TODO: Look for transactions relevant to open storage obligations.

		// Adjust the height of the host. The host height is initialized to
		// zero, but the genesis block is actually height zero. For the genesis
		// block only, the height will be left at zero.
		//
		// Checking the height here eliminates the need to initialize the host
		// to and underflowed types.BlockHeight.
		if block.ID() != types.GenesisBlock.ID() {
			h.blockHeight++
		}

		// TODO: Handle any action items relevant to the current height.
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
