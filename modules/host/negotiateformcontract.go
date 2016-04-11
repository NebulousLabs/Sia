package host

// TODO: Enforce some limit on the percent added that the transaction
// signatures can be to manage fees. Maybe limit the total number of
// signatures, or use some other method to guarantee safety.

// TODO: Host needs some way to prevent renters from making multiple file
// contracts, such that the collateral in the host cannot be drained by a
// single malicious renter. A good defense may include having a limited amount
// of collateral per day that can be used up. The contract cost is a good
// secondary defense. Limit 1 per ip address is a thought, though you get in
// trouble with shared spaces... =/
//
// I guess you can ban any renter that's not using the storage correctly, or at
// least throw down a temporary ban.
//
// The host could exponentially increase the contract price as the amount of
// collateral that the host has available decreases.

// TODO: Test the safety of the builder, it should be okay to have multiple
// builders open for up to 600 seconds, which means multiple blocks could be
// received in that time period.

// TODO: Would be nice to have some sort of error transport to the user, so
// that the user is notified in ways other than logs via the host that there
// are issues such as disk, etc.

// TODO: Write tests where the renter supplies nil values from over the wire
// where possible.

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
	// errBadContractUnlockHash is returned when the host receives a file
	// contract where it does not understand the unlock hash driving the
	// contract.
	errBadContractUnlockHash = errors.New("file contract has an unexpected unlock hash")

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

	// errNilFileContractTransactionSet is returned if the renter provides a
	// nil file contract transaction set during file contract negotiation.
	errNilFileContractTransactionSet = errors.New("file contract transaction set is nil - invalid!")

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
func contractCollateral(settings modules.HostInternalSettings, txnSet []types.Transaction) types.Currency {
	// The host adds collateral based on the host settings and based on how
	// much funding the renter has put into the contract. The determination is
	// made by looking at the payout of the file contract, and assuming that
	// based on the settings the renter has correctly predicted the amount of
	// coins that the host would try to add as collateral.
	fc := txnSet[len(txnSet)-1].FileContracts[0]
	singlePortion := fc.Payout.Div(settings.CollateralFraction.Add(types.NewCurrency64(1e6)))
	hostPortion := singlePortion.Mul(settings.CollateralFraction)
	if hostPortion.Cmp(settings.MaxCollateral) > 0 {
		hostPortion = settings.MaxCollateral
	}
	return hostPortion
}

// managedAddCollateral adds the host's collateral to the file contract
// transaction set, returning the new inputs and outputs that get added to the
// transaction, as well as any new parents that get added to the transaction
// set. The builder that is used to add the collateral is also returned,
// because the new transaction has not yet been signed.
func (h *Host) managedAddCollateral(txnSet []types.Transaction) (builder modules.TransactionBuilder, newParents []types.Transaction, newInputs []types.SiacoinInput, newOutputs []types.SiacoinOutput, err error) {
	// Add the collateral.
	h.mu.RLock()
	settings := h.settings
	h.mu.RUnlock()
	hostPortion := contractCollateral(settings, txnSet)
	txn := txnSet[len(txnSet)-1]
	parents := txnSet[:len(txnSet)-1]
	builder = h.wallet.RegisterTransaction(txn, parents)
	err = builder.FundSiacoins(hostPortion)
	if err != nil {
		builder.Drop()
		return nil, nil, nil, nil, err
	}

	// Return which inputs and outputs have been added by the collateral call.
	newParentIndices, newInputIndices, _, _ := builder.ViewAdded()
	updatedTxn, updatedParents := builder.View()
	for _, parentIndex := range newParentIndices {
		newParents = append(newParents, updatedParents[parentIndex])
	}
	for _, inputIndex := range newInputIndices {
		newInputs = append(newInputs, updatedTxn.SiacoinInputs[inputIndex])
	}
	return builder, newParents, newInputs, nil, nil
}

// managedFinalizeContract will take a file contract, add the host's
// collateral, and then try submitting the file contract to the transaction
// pool. If there is no error, the completed transaction set will be returned
// to the caller.
func (h *Host) managedFinalizeContract(builder modules.TransactionBuilder, renterSignatures []types.TransactionSignature) ([]types.TransactionSignature, error) {
	for _, sig := range renterSignatures {
		builder.AddTransactionSignature(sig)
	}
	fullTxnSet, err := builder.Sign(true)
	if err != nil {
		builder.Drop()
		return nil, err
	}

	// Submit the transaction to the transaction pool, and then return the full
	// transaction set.
	err = h.tpool.AcceptTransactionSet(fullTxnSet)
	if err != nil {
		builder.Drop()
		return nil, err
	}

	// Create and add the storage obligation for this file contract.
	h.mu.Lock()
	defer h.mu.Unlock()
	fullTxn, parentTxns := builder.View()
	hostPortion := contractCollateral(h.settings, append(parentTxns, fullTxn))
	so := &storageObligation{
		ConfirmedRevenue:     h.settings.MinimumContractPrice,
		LockedCollateral:     hostPortion,
		OriginTransactionSet: fullTxnSet,
	}
	err = h.addStorageObligation(so)
	if err != nil {
		// An error here is pretty bad, because the signed file contract has
		// already been broadcast to the world, meaning the host is going to be
		// bleeding money. The host should stop accepting contracts so that the
		// damage can be controlled.
		h.log.Println(err)
		h.settings.AcceptingContracts = false
		builder.Drop()
		return nil, err
	}

	// Get the host's transaction signatures from the builder.
	var hostTxnSignatures []types.TransactionSignature
	_, _, _, txnSigIndices := builder.ViewAdded()
	for _, sigIndex := range txnSigIndices {
		hostTxnSignatures = append(hostTxnSignatures, fullTxn.TransactionSignatures[sigIndex])
	}
	return hostTxnSignatures, nil
}

// managedRPCFormContract accepts a file contract from a renter, checks the
// file contract for compliance with the host settings, and then commits to the
// file contract, creating a storage obligation and submitting the contract to
// the blockchain.
func (h *Host) managedRPCFormContract(conn net.Conn) error {
	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateFileContractTime))

	// Send the host settings to the renter. If the host is not accepting new
	// contracts, the renter is expected to see this and gracefully handle the
	// host closing the connection.
	h.mu.RLock()
	settings := h.settings
	secretKey := h.secretKey
	h.mu.RUnlock()
	err := crypto.WriteSignedObject(conn, settings, secretKey)
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
	var acceptStr string
	err = encoding.ReadObject(conn, &acceptStr, modules.MaxErrorSize)
	if err != nil {
		return err
	}
	if acceptStr != modules.AcceptResponse {
		return errors.New(acceptStr)
	}

	// The renter has sent an indication that the settings are acceptable, and
	// is now going to send a signed file contract that funds the renter's
	// portion of the file contract, including any parent transactions.
	var txnSet []types.Transaction
	var renterPK crypto.PublicKey
	err = encoding.ReadObject(conn, &txnSet, modules.MaxFileContractSetLen)
	if err != nil {
		return err
	}
	err = encoding.ReadObject(conn, &renterPK, modules.MaxFileContractSetLen)
	if err != nil {
		return err
	}

	// The host verifies that the file contract coming over the wire is
	// acceptable.
	err = h.managedVerifyNewContract(txnSet, renterPK)
	if err != nil {
		// The incoming file contract is not acceptable to the host, indicate
		// why to the renter.
		return rejectNegotiation(conn, err)
	}
	// The host adds collateral, then sends any new parent transactions,
	// followed by any new inputs to the transaction, followed by any new
	// outputs to the transaction.
	txnBuilder, newParents, newInputs, newOutputs, err := h.managedAddCollateral(txnSet)
	if err != nil {
		return rejectNegotiation(conn, err)
	}
	err = encoding.WriteObject(conn, newParents)
	if err != nil {
		return err
	}
	err = encoding.WriteObject(conn, newInputs)
	if err != nil {
		return err
	}
	err = encoding.WriteObject(conn, newOutputs)
	if err != nil {
		return err
	}

	// The renter will now send either an acceptance or rejection, followed by
	// a transaction signature in the case of acceptance.
	err = encoding.ReadObject(conn, &acceptStr, modules.MaxErrorSize)
	if err != nil {
		return err
	}
	if acceptStr != modules.AcceptResponse {
		return errors.New(acceptStr)
	}
	var renterTxnSignatures []types.TransactionSignature
	err = encoding.ReadObject(conn, &renterTxnSignatures, 5e3)
	if err != nil {
		return err
	}

	// The host adds the renter transaction signatures, then signs the
	// transaction and submits it to the blockchain, creating a storage
	// obligation in the process. The host's part is done before anything is
	// written to the renter, but to give the renter confidence, the host will
	// send the signatures so that the renter can immediately have the
	// completed file contract.
	hostTxnSignatures, err := h.managedFinalizeContract(txnBuilder, renterTxnSignatures)
	if err != nil {
		// The incoming file contract is not acceptable to the host, indicate
		// why to the renter.
		return rejectNegotiation(conn, err)
	}

	// The host sends the transaction signatures to the renter. Negotiation is
	// complete.
	return encoding.WriteObject(conn, hostTxnSignatures)
}

// managedVerifyNewContract checks that an incoming file contract matches the host's
// expectations for a valid contract.
func (h *Host) managedVerifyNewContract(txnSet []types.Transaction, renterPK crypto.PublicKey) error {
	// Check that the transaction set is not nil - a nil transaction set could
	// cause a panic and is therefore not allowed.
	if txnSet == nil {
		return errNilFileContractTransactionSet
	}

	h.mu.RLock()
	blockHeight := h.blockHeight
	publicKey := h.publicKey
	settings := h.settings
	unlockHash := h.unlockHash
	h.mu.RUnlock()
	fc := txnSet[len(txnSet)-1].FileContracts[0]

	// A new file contract should have a file size of zero.
	if fc.FileSize != 0 {
		return errNonEmptyFC
	}
	// WindowStart must be at least revisionSubmissionBuffer blocks into the
	// future.
	if fc.WindowStart <= blockHeight+revisionSubmissionBuffer {
		return errWindowStartTooSoon
	}
	// WindowEnd must be at least settings.WindowSize blocks after WindowStart.
	if fc.WindowStart+settings.WindowSize >= fc.WindowEnd {
		return errWindowSizeTooSmall
	}
	// ValidProofOutputs and MissedProofOutputs must both have len(2).
	if len(fc.ValidProofOutputs) != 2 || len(fc.MissedProofOutputs) != 2 {
		return errBadPayoutsLen
	}
	// The valid proof outputs and missed proof outputs for the host (index 1)
	// must both have payouts that cover the 'ContractPrice' plus the expected
	// host collateral.
	hostPortion := contractCollateral(settings, txnSet)
	if fc.ValidProofOutputs[1].Value.Cmp(settings.MinimumContractPrice.Add(hostPortion)) != 0 || fc.MissedProofOutputs[1].Value.Cmp(settings.MinimumContractPrice.Add(hostPortion)) != 0 {
		return errBadPayoutsAmounts
	}
	// The unlock hashes of the valid and missed proof outputs for the host
	// must match the host's unlock hash.
	if fc.ValidProofOutputs[1].UnlockHash != unlockHash || fc.MissedProofOutputs[1].UnlockHash != unlockHash {
		return errBadPayoutsUnlockHashes
	}

	// The unlock hash for the file contract must match the unlock hash that
	// the host knows how to spend.
	expectedUH := types.UnlockConditions{
		PublicKeys: []types.SiaPublicKey{
			{
				Algorithm: types.SignatureEd25519,
				Key:       renterPK[:],
			},
			{
				Algorithm: types.SignatureEd25519,
				Key:       publicKey.Key,
			},
		},
		SignaturesRequired: 2,
	}.UnlockHash()
	if fc.UnlockHash != expectedUH {
		return errBadContractUnlockHash
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
