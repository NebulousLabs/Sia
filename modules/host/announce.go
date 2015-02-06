package host

import (
	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/network"
)

// Announce creates a host announcement transaction, adding information to the
// arbitrary data, signing the transaction, and submitting it to the
// transaction pool.
func (h *Host) Announce(addr network.Address, freezeVolume consensus.Currency, freezeDuration consensus.BlockHeight) (err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// get current state height
	h.state.RLock()
	freezeUnlockHeight := h.state.Height() + freezeDuration
	h.state.RUnlock()

	// create the transaction that will hold the announcement
	var t consensus.Transaction
	id, err := h.wallet.RegisterTransaction(t)
	if err != nil {
		return
	}
	err = h.wallet.FundTransaction(id, freezeVolume)
	if err != nil {
		return
	}
	spendHash, spendConditions, err := h.wallet.TimelockedCoinAddress(freezeUnlockHeight)
	if err != nil {
		return
	}
	output := consensus.SiacoinOutput{
		Value:     freezeVolume,
		SpendHash: spendHash,
	}
	freezeIndex, err := h.wallet.AddOutput(id, output)
	if err != nil {
		return
	}

	// create and encode the announcement
	announcement := encoding.Marshal(modules.HostAnnouncement{
		IPAddress:       addr,
		FreezeIndex:     freezeIndex,
		SpendConditions: spendConditions,
	})

	// add announcement to arbitrary data field
	err = h.wallet.AddArbitraryData(id, modules.HostAnnouncementPrefix+string(announcement))
	if err != nil {
		return
	}
	// TODO: Have the wallet manually add a fee? How should this be managed?
	t, err = h.wallet.SignTransaction(id, true)
	if err != nil {
		return
	}

	h.tpool.AcceptTransaction(t)

	return
}
