package host

import (
	"errors"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// Announce creates a host announcement transaction, adding information to the
// arbitrary data, signing the transaction, and submitting it to the
// transaction pool.
func (h *Host) Announce() error {
	// create the transaction that will hold the announcement
	var t types.Transaction
	id, err := h.wallet.RegisterTransaction(t)
	if err != nil {
		return err
	}

	// create and encode the announcement and add it to the arbitrary data of
	// the transaction.
	lockID := h.mu.RLock()
	if h.myAddr == "" {
		h.mu.RUnlock(lockID)
		return errors.New("can't announce without knowing external IP")
	}
	announcement := encoding.Marshal(modules.HostAnnouncement{
		IPAddress: h.myAddr,
	})
	h.mu.RUnlock(lockID)
	_, _, err = h.wallet.AddArbitraryData(id, modules.PrefixHostAnnouncement+string(announcement))
	if err != nil {
		return err
	}
	t, err = h.wallet.SignTransaction(id, true)
	if err != nil {
		return err
	}

	// Add the transaction to the transaction pool.
	err = h.tpool.AcceptTransaction(t)
	if err != nil {
		return err
	}

	return nil
}
