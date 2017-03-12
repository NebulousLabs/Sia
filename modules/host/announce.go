package host

import (
	"errors"

	"github.com/NebulousLabs/Sia/modules"
)

var (
	// errAnnWalletLocked is returned during a host announcement if the wallet
	// is locked.
	errAnnWalletLocked = errors.New("cannot announce the host while the wallet is locked")

	// errUnknownAddress is returned if the host is unable to determine a
	// public address for itself to use in the announcement.
	errUnknownAddress = errors.New("host cannot announce, does not seem to have a valid address.")
)

// managedAnnounce creates an announcement transaction and submits it to the network.
func (h *Host) managedAnnounce(addr modules.NetAddress) error {
	// The wallet needs to be unlocked to add fees to the transaction, and the
	// host needs to have an active unlock hash that renters can make payment
	// to.
	if !h.wallet.Unlocked() {
		return errAnnWalletLocked
	}

	h.mu.Lock()
	pubKey := h.publicKey
	secKey := h.secretKey
	err := h.checkUnlockHash()
	h.mu.Unlock()
	if err != nil {
		return err
	}

	// Create the announcement that's going to be added to the arbitrary data
	// field of the transaction.
	signedAnnouncement, err := modules.CreateAnnouncement(addr, pubKey, secKey)
	if err != nil {
		return err
	}

	// Create a transaction, with a fee, that contains the full announcement.
	txnBuilder := h.wallet.StartTransaction()
	_, fee := h.tpool.FeeEstimation()
	fee = fee.Mul64(500) // Estimated txn size (in bytes) of a host announcement.
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

	h.mu.Lock()
	h.announced = true
	h.mu.Unlock()
	h.log.Printf("INFO: Successfully announced as %v", addr)
	return nil
}

// Announce creates a host announcement transaction, adding information to the
// arbitrary data, signing the transaction, and submitting it to the
// transaction pool.
func (h *Host) Announce() error {
	err := h.tg.Add()
	if err != nil {
		return err
	}
	defer h.tg.Done()

	// Determine whether to use the settings.NetAddress or autoAddress.
	h.mu.RLock()
	na := h.settings.NetAddress
	aa := h.autoAddress
	h.mu.RUnlock()
	if na != "" {
		return h.managedAnnounce(na)
	}
	if aa == "" {
		return errUnknownAddress
	}
	return h.managedAnnounce(aa)
}

// AnnounceAddress submits a host announcement to the blockchain to announce a
// specific address. No checks for validity are performed on the address.
func (h *Host) AnnounceAddress(addr modules.NetAddress) error {
	err := h.tg.Add()
	if err != nil {
		return err
	}
	defer h.tg.Done()
	return h.managedAnnounce(addr)
}
