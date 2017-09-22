package host

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
	"github.com/NebulousLabs/fastrand"

	"github.com/NebulousLabs/bolt"
)

var (
	// errRevisionWrongPublicKeyCount is returned when a stored file contract
	// revision does not have enough public keys - such a situation should
	// never happen, and is a critical / developer error.
	errRevisionWrongPublicKeyCount = errors.New("wrong number of public keys in the unlock conditions of the file contract revision")

	// errVerifyChallenge is returned to renter instead of any error
	// returned by managedVerifyChallengeResponse. It is used instead
	// of the original error not to leak if the host has the contract
	// with the ID sent by renter.
	errVerifyChallenge = errors.New("bad signature from renter or no such contract")
)

// managedVerifyChallengeResponse will verify that the renter's response to the
// challenge provided by the host is correct. In the process, the storage
// obligation and file contract revision will be loaded and returned.
//
// The storage obligation is returned under a storage obligation lock.
func (h *Host) managedVerifyChallengeResponse(fcid types.FileContractID, challenge crypto.Hash, challengeResponse crypto.Signature) (so storageObligation, recentRevision types.FileContractRevision, revisionSigs []types.TransactionSignature, err error) {
	// Grab a lock before it is possible to perform any operations on the
	// storage obligation. Defer a call to unlock in the event of an error. If
	// there is no error, the storage obligation will be returned with a lock.
	err = h.managedTryLockStorageObligation(fcid)
	if err != nil {
		err = extendErr("could not get "+fcid.String()+" lock: ", ErrorInternal(err.Error()))
		return storageObligation{}, types.FileContractRevision{}, nil, err
	}
	defer func() {
		if err != nil {
			h.managedUnlockStorageObligation(fcid)
		}
	}()

	// Fetch the storage obligation, which has the revision, which has the
	// renter's public key.
	h.mu.RLock()
	defer h.mu.RUnlock()
	err = h.db.View(func(tx *bolt.Tx) error {
		so, err = getStorageObligation(tx, fcid)
		return err
	})
	if err != nil {
		err = extendErr("could not fetch "+fcid.String()+": ", ErrorInternal(err.Error()))
		return storageObligation{}, types.FileContractRevision{}, nil, err
	}

	// Pull out the file contract revision and the revision's signatures from
	// the transaction.
	revisionTxn := so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1]
	recentRevision = revisionTxn.FileContractRevisions[0]
	for _, sig := range revisionTxn.TransactionSignatures {
		// Checking for just the parent id is sufficient, an over-signed file
		// contract is invalid.
		if sig.ParentID == crypto.Hash(fcid) {
			revisionSigs = append(revisionSigs, sig)
		}
	}

	// Verify that the challegne response matches the public key.
	var renterPK crypto.PublicKey
	// Sanity check - there should be two public keys.
	if len(recentRevision.UnlockConditions.PublicKeys) != 2 {
		// The error has to be set here so that the defered error check will
		// unlock the storage obligation.
		h.log.Critical("wrong public key count in file contract revision")
		err = errRevisionWrongPublicKeyCount
		err = extendErr("wrong public key count for "+fcid.String()+": ", ErrorInternal(err.Error()))
		return storageObligation{}, types.FileContractRevision{}, nil, err
	}
	copy(renterPK[:], recentRevision.UnlockConditions.PublicKeys[0].Key)
	err = crypto.VerifyHash(challenge, renterPK, challengeResponse)
	if err != nil {
		err = extendErr("bad signature from renter: ", ErrorCommunication(err.Error()))
		return storageObligation{}, types.FileContractRevision{}, nil, err
	}
	return so, recentRevision, revisionSigs, nil
}

// managedRPCRecentRevision sends the most recent known file contract
// revision, including signatures, to the renter, for the file contract with
// the id given by the renter.
//
// The storage obligation is returned under a storage obligation lock.
func (h *Host) managedRPCRecentRevision(conn net.Conn) (types.FileContractID, storageObligation, error) {
	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateRecentRevisionTime))

	// Receive the file contract id from the renter.
	var fcid types.FileContractID
	err := encoding.ReadObject(conn, &fcid, uint64(len(fcid)))
	if err != nil {
		return types.FileContractID{}, storageObligation{}, extendErr("could not read file contract id: ", ErrorConnection(err.Error()))
	}

	// Send a challenge to the renter to verify that the renter has write
	// access to the revision being opened.
	var challenge crypto.Hash
	fastrand.Read(challenge[16:])
	err = encoding.WriteObject(conn, challenge)
	if err != nil {
		return types.FileContractID{}, storageObligation{}, extendErr("cound not write challenge: ", ErrorConnection(err.Error()))
	}

	// Read the signed response from the renter.
	var challengeResponse crypto.Signature
	err = encoding.ReadObject(conn, &challengeResponse, uint64(len(challengeResponse)))
	if err != nil {
		return types.FileContractID{}, storageObligation{}, extendErr("could not read challenge response: ", ErrorConnection(err.Error()))
	}
	// Verify the response. In the process, fetch the related storage
	// obligation, file contract revision, and transaction signatures.
	so, recentRevision, revisionSigs, err := h.managedVerifyChallengeResponse(fcid, challenge, challengeResponse)
	if err != nil {
		// Do not disclose the original error to renter not to leak
		// if the host has the contract with the ID sent by renter.
		modules.WriteNegotiationRejection(conn, errVerifyChallenge)
		return types.FileContractID{}, storageObligation{}, extendErr("challenge failed: ", err)
	}
	// Defer a call to unlock the storage obligation in the event of an error.
	defer func() {
		if err != nil {
			h.managedUnlockStorageObligation(fcid)
		}
	}()

	// Send the file contract revision and the corresponding signatures to the
	// renter.
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		err = extendErr("failed to write challenge acceptance: ", ErrorConnection(err.Error()))
		return types.FileContractID{}, storageObligation{}, err
	}
	err = encoding.WriteObject(conn, recentRevision)
	if err != nil {
		err = extendErr("failed to write recent revision: ", ErrorConnection(err.Error()))
		return types.FileContractID{}, storageObligation{}, err
	}
	err = encoding.WriteObject(conn, revisionSigs)
	if err != nil {
		err = extendErr("failed to write recent revision signatures: ", ErrorConnection(err.Error()))
		return types.FileContractID{}, storageObligation{}, err
	}
	return fcid, so, nil
}
