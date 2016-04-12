package host

import (
	"errors"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

var (
	// errAnnWalletLocked is returned during a host announcement if the wallet
	// is locked.
	errAnnWalletLocked = errors.New("cannot announce the host while the wallet is locked")
)

// announce creates an announcement transaction and submits it to the network.
func (h *Host) announce(addr modules.NetAddress) error {
	if !h.wallet.Unlocked() {
		return errAnnWalletLocked
	}
	err := h.checkUnlockHash()
	if err != nil {
		return err
	}

	// Create a host announcement and a signature for the announcement.
	announcement := encoding.Marshal(modules.HostAnnouncement{
		NetAddress: addr,
		PublicKey:  h.publicKey,
	})
	annHash := crypto.HashBytes(announcement)
	sig, err := crypto.SignHash(annHash, h.secretKey)
	if err != nil {
		return err
	}
	annAndSig := append(sig[:], announcement...)

	// Create a transaction, with a fee, that contains both the announcement
	// and signature.
	txnBuilder := h.wallet.StartTransaction()
	_, fee := h.tpool.FeeEstimation()
	err = txnBuilder.FundSiacoins(fee)
	if err != nil {
		txnBuilder.Drop()
		return err
	}
	_ = txnBuilder.AddMinerFee(fee)
	_ = txnBuilder.AddArbitraryData(append(modules.PrefixHostAnnouncement[:], annAndSig...))
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		txnBuilder.Drop()
		return err
	}

	// Add the transactions to the transaction pool.
	err = h.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return err
	}
	h.log.Printf("INFO: Successfully announced as %v", addr)

	// Start accepting contracts.
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
