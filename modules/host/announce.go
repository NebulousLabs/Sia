package host

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Announce creates a host announcement transaction, adding information to the
// arbitrary data, signing the transaction, and submitting it to the
// transaction pool.
func (h *Host) Announce(addr modules.NetAddress) (err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// create the transaction that will hold the announcement
	var t types.Transaction
	id, err := h.wallet.RegisterTransaction(t)
	if err != nil {
		return
	}

	// create and encode the announcement and add it to the arbitrary data of
	// the transaction.
	announcement := encoding.Marshal(modules.HostAnnouncement{
		IPAddress: addr,
	})
	_, _, err = h.wallet.AddArbitraryData(id, modules.PrefixHostAnnouncement+string(announcement))
	if err != nil {
		return
	}
	t, err = h.wallet.SignTransaction(id, true)
	if err != nil {
		return
	}

	// Add the transaction to the transaction pool.
	err = h.tpool.AcceptTransaction(t)
	if err != nil {
		return
	}

	return
}
