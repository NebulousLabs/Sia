package host

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// announce creates an announcement transaction and submits it to the network.
func (h *Host) announce(addr modules.NetAddress) error {
	// Generate an unlock hash, if necessary.
	if h.settings.UnlockHash == (types.UnlockHash{}) {
		uc, err := h.wallet.NextAddress()
		if err != nil {
			return err
		}
		h.settings.UnlockHash = uc.UnlockHash()
		err = h.save()
		if err != nil {
			return err
		}
	}

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
	h.log.Printf("INFO: Successfully announced as %v", addr)

	h.acceptingContracts = true
	return nil
}

// Announce creates a host announcement transaction, adding information to the
// arbitrary data, signing the transaction, and submitting it to the
// transaction pool.
func (h *Host) Announce() error {
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if h.closed {
		return errHostClosed
	}

	// Get the external IP again; it may have changed.
	h.learnHostname()
	h.mu.RLock()
	addr := h.netAddress
	h.mu.RUnlock()

	// Check that the host's ip address is known.
	if addr.IsLoopback() && build.Release != "testing" {
		return errors.New("can't announce without knowing external IP")
	}

	return h.announce(addr)
}

// AnnounceAddress submits a host announcement to the blockchain to announce a
// specific address. No checks for validity are performed on the address.
func (h *Host) AnnounceAddress(addr modules.NetAddress) error {
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if h.closed {
		return errHostClosed
	}
	return h.announce(addr)
}
