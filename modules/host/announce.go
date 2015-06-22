package host

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	pingTimeout = 10 * time.Second
)

// ping establishes a connection to addr and then immediately closes it. It is
// used to verify that an address is connectible.
func ping(addr modules.NetAddress) bool {
	conn, err := net.DialTimeout("tcp", string(addr), pingTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// announce creates an announcement transaction and submits it to the network.
func (h *Host) announce(addr modules.NetAddress) error {
	// create the transaction that will hold the announcement
	var t types.Transaction
	id, err := h.wallet.RegisterTransaction(t)
	if err != nil {
		return err
	}

	// create and encode the announcement and add it to the arbitrary data of
	// the transaction.
	announcement := encoding.Marshal(modules.HostAnnouncement{
		IPAddress: addr,
	})
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
	if err == modules.ErrTransactionPoolDuplicate {
		return errors.New("you have already announced yourself")
	}
	if err != nil {
		return err
	}

	return nil
}

// Announce creates a host announcement transaction, adding information to the
// arbitrary data, signing the transaction, and submitting it to the
// transaction pool.
func (h *Host) Announce() error {
	// check that our address is reachable
	lockID := h.mu.RLock()
	addr := h.myAddr
	h.mu.RUnlock(lockID)
	if addr.Host() == "" {
		return errors.New("can't announce without knowing external IP")
	} else if !ping(addr) {
		return errors.New("host address not reachable; ensure you have forwarded port " + addr.Port())
	}
	return h.announce(addr)
}

// ForceAnnounce skips the check for knowing your external IP and for checking
// your port.
func (h *Host) ForceAnnounce() error {
	lockID := h.mu.RLock()
	addr := h.myAddr
	h.mu.RUnlock(lockID)
	return h.announce(addr)
}
