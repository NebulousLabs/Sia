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
func (h *Host) Announce(addr network.Address) (err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// create the transaction that will hold the announcement
	var t consensus.Transaction
	id, err := h.wallet.RegisterTransaction(t)
	if err != nil {
		return
	}

	// create and encode the announcement and add it to the arbitrary data of
	// the transaction.
	announcement := encoding.Marshal(modules.HostAnnouncement{
		IPAddress: addr,
	})
	err = h.wallet.AddArbitraryData(id, modules.PrefixHostAnnouncement+string(announcement))
	if err != nil {
		return
	}
	t, err = h.wallet.SignTransaction(id, true)
	if err != nil {
		return
	}

	h.tpool.AcceptTransaction(t)

	return
}
