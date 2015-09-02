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
	// Create a transaction with a host announcement.
	txnBuilder := h.wallet.StartTransaction()
	announcement := encoding.Marshal(modules.HostAnnouncement{
		IPAddress: addr,
	})
	_ = txnBuilder.AddArbitraryData(append(modules.PrefixHostAnnouncement[:], announcement...))
	txn, parents := txnBuilder.View()
	txnSet := append(parents, txn)

	// Add the transaction to the transaction pool.
	err := h.tpool.AcceptTransactionSet(txnSet)
	if err == modules.ErrDuplicateTransactionSet {
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
	// Generate an unlock hash, if necessary
	if h.UnlockHash == (types.UnlockHash{}) {
		uc, err := h.wallet.NextAddress()
		if err != nil {
			return err
		}
		h.UnlockHash = uc.UnlockHash()
		err = h.save()
		if err != nil {
			return err
		}
	}

	// Get the external IP again; it may have changed.
	h.learnHostname()
	lockID := h.mu.RLock()
	addr := h.myAddr
	h.mu.RUnlock(lockID)

	// Check that the host's ip address is both known and reachable.
	if addr.Host() == "::1" {
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
