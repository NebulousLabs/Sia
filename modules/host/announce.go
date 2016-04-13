package host

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	// errAnnWalletLocked is returned during a host announcement if the wallet
	// is locked.
	errAnnWalletLocked = errors.New("cannot announce the host while the wallet is locked")
)

// announce creates an announcement transaction and submits it to the network.
func (h *Host) announce(addr modules.NetAddress) error {
	// The wallet needs to be unlocked to add fees to the transaction, and the
	// host needs to have an active unlock hash that renters can make payment
	// to.
	if !h.wallet.Unlocked() {
		return errAnnWalletLocked
	}
	err := h.checkUnlockHash()
	if err != nil {
		return err
	}

	// Create the announcement that's going to be added to the arbitrary data
	// field of the transaction.
	signedAnnouncement, err := modules.CreateAnnouncement(addr, h.publicKey, h.secretKey)
	if err != nil {
		return err
	}

	// Create a transaction, with a fee, that contains the full announcement.
	txnBuilder := h.wallet.StartTransaction()
	_, fee := h.tpool.FeeEstimation()
	err = txnBuilder.FundSiacoins(fee)
	if err != nil {
		txnBuilder.Drop()
		return err
	}
	_ = txnBuilder.AddMinerFee(fee)
	_ = txnBuilder.AddArbitraryData(signedAnnouncement)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		txnBuilder.Drop()
		return err
	}

	// Add the transactions to the transaction pool.
	err = h.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		txnBuilder.Drop()
		return err
	}
	h.log.Printf("INFO: Successfully announced as %v", addr)

	// Start accepting contracts.
	//
	// TODO: I'm not sure that we should keep this auto-accept feature. If the
	// host is having significant disk trouble, it'll shut down. The user
	// shouldn't be making announcements while the host is having disk trouble,
	// but I still worry that the user will be turning on the host on accident
	// sometimes. Furthermore, there's not a clear relationship between making
	// an announcement and accepting file contracts, at least not one that's
	// explicit. A host may want to announce before being ready to accept file
	// contracts that way it's uptime clock and long-term clock can begin.
	h.settings.AcceptingContracts = true
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

	h.mu.Lock()
	defer h.mu.Unlock()
	return h.announce(addr)
}

// AnnounceAddress submits a host announcement to the blockchain to announce a
// specific address. No checks for validity are performed on the address.
func (h *Host) AnnounceAddress(addr modules.NetAddress) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resourceLock.RLock()
	defer h.resourceLock.RUnlock()
	if h.closed {
		return errHostClosed
	}

	h.revisionNumber++
	return h.announce(addr)
}
