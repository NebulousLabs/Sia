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

	// errUnknownAddress is returned if the host is unable to determine a
	// public address for itself to use in the announcement.
	errUnknownAddress = errors.New("host cannot announce, does not seem to have a valid address")
)

// managedAnnounce creates an announcement transaction and submits it to the network.
func (h *Host) managedAnnounce(addr modules.NetAddress) (err error) {
	// The wallet needs to be unlocked to add fees to the transaction, and the
	// host needs to have an active unlock hash that renters can make payment
	// to.
	unlocked, err := h.wallet.Unlocked()
	if err != nil {
		return err
	}
	if !unlocked {
		return errAnnWalletLocked
	}

	h.mu.Lock()
	pubKey := h.publicKey
	secKey := h.secretKey
	err = h.checkUnlockHash()
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
	txnBuilder, err := h.wallet.StartTransaction()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			txnBuilder.Drop()
		}
	}()
	_, fee := h.tpool.FeeEstimation()
	fee = fee.Mul64(600) // Estimated txn size (in bytes) of a host announcement.
	err = txnBuilder.FundSiacoins(fee)
	if err != nil {
		return err
	}
	_ = txnBuilder.AddMinerFee(fee)
	_ = txnBuilder.AddArbitraryData(signedAnnouncement)
	txnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return err
	}

	// Add the transactions to the transaction pool.
	err = h.tpool.AcceptTransactionSet(txnSet)
	if err != nil {
		return err
	}

	h.mu.Lock()
	h.announced = true
	h.mu.Unlock()
	h.log.Printf("INFO: Successfully announced as %v", addr)
	return nil
}

// Announce creates a host announcement transaction.
func (h *Host) Announce() error {
	err := h.tg.Add()
	if err != nil {
		return err
	}
	defer h.tg.Done()

	// Grab the internal net address and internal auto address, and compare
	// them.
	h.mu.RLock()
	userSet := h.settings.NetAddress
	autoSet := h.autoAddress
	h.mu.RUnlock()

	// Check that we have at least one address to work with.
	if userSet == "" && autoSet == "" {
		return build.ExtendErr("cannot announce because address could not be determined", err)
	}

	// Prefer using the userSet address, otherwise use the automatic address.
	var annAddr modules.NetAddress
	if userSet != "" {
		annAddr = userSet
	} else {
		annAddr = autoSet
	}

	// Check that the address is sane, and that the address is also not local.
	err = annAddr.IsStdValid()
	if err != nil {
		return build.ExtendErr("announcement requested with bad net address", err)
	}
	if annAddr.IsLocal() && build.Release != "testing" {
		return errors.New("announcement requested with local net address")
	}

	// Address has cleared inspection, perform the announcement.
	return h.managedAnnounce(annAddr)
}

// AnnounceAddress submits a host announcement to the blockchain to announce a
// specific address. If there is no error, the host's address will be updated
// to the supplied address.
func (h *Host) AnnounceAddress(addr modules.NetAddress) error {
	err := h.tg.Add()
	if err != nil {
		return err
	}
	defer h.tg.Done()

	// Check that the address is sane, and that the address is also not local.
	err = addr.IsStdValid()
	if err != nil {
		return build.ExtendErr("announcement requested with bad net address", err)
	}
	if addr.IsLocal() {
		return errors.New("announcement requested with local net address")
	}

	// Attempt the actual announcement.
	err = h.managedAnnounce(addr)
	if err != nil {
		return build.ExtendErr("unable to perform manual host announcement", err)
	}

	// Address is valid, update the host's internal net address to match the
	// specified addr.
	h.mu.Lock()
	h.settings.NetAddress = addr
	h.mu.Unlock()
	return nil
}
