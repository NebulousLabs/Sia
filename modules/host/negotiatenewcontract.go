package host

// TODO: Concurrency is not properly managed in this file.

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// errBadPayoutsLen is returned if a new file contract is presented that
	// has the wrong number of valid or missed proof payouts.
	errBadPayoutsLen = errors.New("file contract has the wrong number of payouts - there should be two valid and two missed payouts")

	// errBadPayoutsAmounts is returned if a new file contract is presented that
	// does not pay the correct amount to the host - by default, the payouts
	// should be paying the contract price.
	errBadPayoutsAmounts = errors.New("file contract has payouts that do not correctly cover the contract price")

	// errBadPayoutsUnlockHashes is returned if a new file contract is
	// presented that does not make payments to the correct addresses.
	errBadPayoutsUnlockHashes = errors.New("file contract has payouts which pay to the wrong unlock hashes for the host")

	// errLowFees is returned if a transaction set provided by the renter does
	// not have large enough transaction fees to have a reasonalbe chance at
	// making it onto the blockchain.
	errLowFees = errors.New("file contract proposal does not have enough transaction fees to be acceptable")

	// errNonEmptyFC is returned if a renter tries to make a new file contract
	// that has a FileSize which is not zero.
	errNonEmptyFC = errors.New("new file contract should have no data in it")

	// errWindowSizeTooSmall is returned if a file contract has a window size
	// (defined by fc.WindowEnd - fc.WindowStart) which is too small to be
	// acceptable to the host - the host needs to submit its storage proof to
	// the blockchain inside of that window.
	errWindowSizeTooSmall = errors.New("file contract has a storage proof window which is not wide enough to match the host's requirements")

	// errWindowStartTooSoon is returned if the storage proof window for the
	// file contract opens too soon into the future - the host needs time to
	// submit the file contract and all revisions to the blockchain before the
	// storage proof window opens.
	errWindowStartTooSoon = errors.New("the storage proof window is opening to soon")
)

// contractCollateral returns the amount of collateral that the host is
// expected to add to the file contract based on the payout of the file
// contract and based on the host settings.
func (h *Host) contractCollateral(txnSet []types.Transaction) types.Currency {
	// The host adds collateral based on the host settings and based on how
	// much funding the renter has put into the contract. The determination is
	// made by looking at the payout of the file contract, and assuming that
	// based on the settings the renter has correctly predicted the amount of
	// coins that the host would try to add as collateral.
	fc := txnSet[len(txnSet)-1].FileContracts[0]
	singlePortion := fc.Payout.Div(h.settings.CollateralFraction.Add(types.NewCurrency64(1e6)))
	hostPortion := singlePortion.Mul(h.settings.CollateralFraction)
	if hostPortion.Cmp(h.settings.MaxCollateral) > 0 {
		hostPortion = h.settings.MaxCollateral
	}
	return hostPortion
}

// finalizeContract will take a file contract, add the host's collateral, and
// then try submitting the file contract to the transaction pool. If there is
// no error, the completed transaction set will be returned to the caller.
func (h *Host) finalizeContract(txnSet []types.Transaction) ([]types.Transaction, error) {
	// Add the collateral.
	hostPortion := h.contractCollateral(txnSet)
	txn := txnSet[len(txnSet)-1]
	parents := txnSet[:len(txnSet)-1]
	builder := h.wallet.RegisterTransaction(txn, parents)
	err = builder.FundSiacoins(hostPortion)
	if err != nil {
		builder.Drop()
		return nil, err
	}
	fullTxnSet, err := builder.Sign(true)
	if err != nil {
		builder.Drop()
		return nil, err
	}

	// Submit the transaction to the transaction pool, and then return the full
	// transaction set.
	err := h.tpool.AcceptTransactionSet(fullTxnSet)
	if err != nil {
		builder.Drop()
		return nil, err
	}

	// Create and add the storage obligation for this file contract.
	so := &storageObligation{
		ConfirmedRevenue: h.settings.ContractPrice,
		LockedCollateral: hostPortion,
		OriginTransactionSet: fullTxnSet,
	}
	err = h.addStorageObligation(so)
	if err != nil {
		builder.Drop()
		return nil, err
	}
	return fullTxnSet, nil
}

// managedRPCFormContract accepts a file contract from a renter, checks the
// file contract for compliance with the host settings, and then commits to the
// file contract, creating a storage obligation and submitting the contract to
// the blockchain.
func (h *Host) managedRPCFormContract(conn net.Conn) error {
	// Allow 120 seconds for negotiation.
	conn.SetDeadline(time.Now().Add(modules.FileContractNegotiationTime))

	// The first thing that the host should do is write the host settings to
	// the connection. If the host is not accepting new contracts, the renter
	// is expected to see this and gracefully handle the host closing the
	// connection.
	h.mu.RLock()
	settings := h.settings
	h.mu.RUnlock()
	err := crypto.WriteSignedObject(conn, settings, h.secretKey)
	if err != nil {
		return err
	}
	if !settings.AcceptingContracts {
		// The host is not accepting contracts, the connection can be closed.
		// The renter has been given enough information to understand that the
		// connection is going to be closed.
		return nil
	}

	// The renter is going to send a string, which will either be an error or
	// will indicate that there was no error.
	var readErr string
	err = encoding.ReadObject(conn, &readErr, modules.MaxErrorSize)
	if err != nil {
		return err
	}
	if readErr != modules.AcceptResponse {
		return errors.New(readErr)
	}

	// The renter has sent an indication that the settings are acceptable, and
	// is now going to send a signed file contract that funds the renter's
	// portion of the file contract, including any parent transactions.
	var txnSet []types.Transaction
	err = encoding.ReadObject(conn, &txnSet, modules.MaxFileContractSetLen)
	if err != nil {
		return err
	}

	// The host verifies that the file contract coming over the wire is
	// acceptable.
	err = h.verifyNewContract(txnSet)
	if err != nil {
		// The incoming file contract is not acceptable to the host, indicate
		// why to the renter.
		writeErr := encoding.WriteObject(conn, err.Error())
		return composeErrors(err, writeErr)
	}

	// The host adds money to the file contract to cover collateral, checks for
	// full validity, and submits the file contract to the transaction pool.
	// The host must perform any storage obligation management.
	fullSet, err := h.finalizeContract(txnSet)
	if err != nil {
		// The incoming file contract is not acceptable to the host, indicate
		// why to the renter.
		writeErr := encoding.WriteObject(conn, err.Error())
		return composeErrors(err, writeErr)
	}

	// The host writes acceptance, and then sends the updated transaction set
	// back to the renter.
	err = encoding.WriteObject(conn, modules.AcceptResponse)
	if err != nil {
		return err
	}
	err = encoding.WriteObject(conn, fullSet)
	if err != nil {
		return err
	}
	// After the send has completed, negotiation is done.
	return nil
}

// verifyNewContract checks that an incoming file contract matches the host's
// expectations for a valid contract.
func (h *Host) verifyNewContract(txnSet []types.Transaction) error {
	fc := txnSet[len(txnSet)-1].FileContracts[0]

	// A new file contract should have a file size of zero.
	if fc.FileSize != 0 {
		return errNonEmptyFC
	}
	// WindowStart must be at least revisionSubmissionBuffer blocks into the
	// future.
	if fc.WindowStart <= h.blockHeight + revisionSubmissionBuffer {
		return errWindowStartTooSoon
	}
	// WindowEnd must be at least settings.WindowSize blocks after WindowStart.
	if fc.WindowStart + h.settings.WindowSize >= fc.WindowEnd {
		return errWindowSizeTooSmall
	}
	// ValidProofOutputs and MissedProofOutputs must both have len(2).
	if len(fc.ValidProofOutputs) != 2 || len(fc.MissedProofOutputs) != 2 {
		return errBadPayoutsLen
	}
	// The valid proof outputs and missed proof outputs for the host (index 1)
	// must both have payouts that cover the 'ContractPrice' plus the expected
	// host collateral.
	hostPortion := h.contractCollateral(txnSet)
	if fc.ValidProofOutputs[1].Value.Cmp(h.settings.ContractPrice.Add(hostPortion)) != 0 || fc.MissedProofOutputs[1].Value.Cmp(h.settings.ContractPrice.Add(hostPortion)) != 0 {
		return errBadPayoutsAmounts
	}
	// The unlock hashes of the valid and missed proof outputs for the host
	// must match the host's unlock hash.
	if fc.ValidProofOutputs[1].UnlockHash != h.unlockHash || fc.MissedProofOutputs[1].UnlockHash != h.unlockHash {
		return errBadPayoutsUnlockHashes
	}

	// Check that the transaction set has enough fees on it to get into the
	// blockchain.
	setFee := modules.CalculateFee(txnSet)
	minFee, _ := h.tpool.FeeEstimation()
	if setFee.Cmp(minFee) < 0 {
		return errLowFees
	}
	return nil
}
