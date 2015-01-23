package host

import (
	"github.com/NebulousLabs/Sia/consensus"
	// "github.com/NebulousLabs/Sia/encoding"
	// "github.com/NebulousLabs/Sia/sia/components"
)

// HostAnnounceSelf creates a host announcement transaction, adding
// information to the arbitrary data and then signing the transaction.
func (h *Host) AnnounceHost(freezeVolume consensus.Currency, freezeUnlockHeight consensus.BlockHeight) (t consensus.Transaction, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	/*
		// Get the encoded announcement based on the host settings.
		info := h.announcement

		// Fill out the transaction.
		id, err := h.wallet.RegisterTransaction(t)
		if err != nil {
			return
		}
		err = h.wallet.FundTransaction(id, freezeVolume)
		if err != nil {
			return
		}
		info.SpendConditions, info.FreezeIndex, err = h.wallet.AddTimelockedRefund(id, freezeVolume, freezeUnlockHeight)
		if err != nil {
			return
		}
		announcement := string(encoding.MarshalAll(components.HostAnnouncementPrefix, info))
		err = h.wallet.AddArbitraryData(id, announcement)
		if err != nil {
			return
		}
		// TODO: Have the wallet manually add a fee? How should this be managed?
		t, err = h.wallet.SignTransaction(id, true)
		if err != nil {
			return
		}

		h.state.AcceptTransaction(t)
	*/

	return
}
