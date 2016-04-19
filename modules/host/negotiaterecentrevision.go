package host

import (
	"crypto/rand"
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

var (
	// errRevisionFewPublicKeys is returned when a stored file contract
	// revision does not have enough public keys - such a situation should
	// never happen, and is a critical / developer error.
	errRevisionFewPublicKeys = errors.New("too few public keys in the unlock conditions of the file contract revision")
)

// verifyChallengeResponse will verify that the renter's response to the
// challenge provided by the host is correct. In the process, the storage
// obligation and file contract revision will be loaded and returned.
func (h *Host) verifyChallengeResponse(fcid types.FileContractID, challenge crypto.Hash, challengeResponse crypto.Signature) (*storageObligation, types.FileContractRevision, []types.TransactionSignature, error) {
	// Fetch the storage obligation, which has the revision, which has the
	// renter's public key.
	var so *storageObligation
	err := h.db.Update(func(tx *bolt.Tx) error {
		fso, err := getStorageObligation(tx, fcid)
		so = &fso
		return err
	})
	if err != nil {
		return nil, types.FileContractRevision{}, nil, err
	}

	// Pull out the file contract revision and the revision's signatures from
	// the transaction.
	revisionTxn := so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1]
	recentRevision := revisionTxn.FileContractRevisions[0]
	var revisionSigs []types.TransactionSignature
	for _, sig := range revisionTxn.TransactionSignatures {
		// Checking for just the parent id is sufficient, an over-signed file
		// contract is invalid.
		if sig.ParentID == crypto.Hash(fcid) {
			revisionSigs = append(revisionSigs, sig)
		}
	}

	// Verify that the challegne response matches public key.
	var renterPK crypto.PublicKey
	// Sanity check - there should be two public keys.
	if len(recentRevision.UnlockConditions.PublicKeys) != 2 {
		h.log.Critical("found a revision with too few public keys")
		return nil, types.FileContractRevision{}, nil, errRevisionFewPublicKeys
	}
	copy(renterPK[:], recentRevision.UnlockConditions.PublicKeys[0].Key)
	err = crypto.VerifyHash(challenge, renterPK, challengeResponse)
	if err != nil {
		return nil, types.FileContractRevision{}, nil, err
	}

	// Sanity check - verify that the host has a valid revision and set of
	// signatures.
	h.mu.RLock()
	blockHeight := h.blockHeight
	h.mu.RUnlock()
	err = modules.VerifyFileContractRevisionTransactionSignatures(recentRevision, revisionSigs, blockHeight)
	if err != nil {
		h.log.Critical("host is inconsistent, bad file contract revision transaction", err)
		return nil, types.FileContractRevision{}, nil, err
	}
	return so, recentRevision, revisionSigs, nil
}

// managedRPCRecentRevision sends the most recent known file contract
// revision, including signatures, to the renter, for the file contract with
// the input id.
func (h *Host) managedRPCRecentRevision(conn net.Conn) (types.FileContractID, *storageObligation, error) {
	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateRecentRevisionTime))

	// Receive the file contract id from the renter.
	var fcid types.FileContractID
	err := encoding.ReadObject(conn, &fcid, uint64(len(fcid)))
	if err != nil {
		return types.FileContractID{}, nil, err
	}

	// Send a challenge to the renter to verify that the renter has write
	// access to the revision being opened.
	var challenge crypto.Hash
	_, err = rand.Read(challenge[:])
	if err != nil {
		return types.FileContractID{}, nil, modules.WriteNegotiationRejection(conn, err)
	}
	err = encoding.WriteObject(conn, challenge)
	if err != nil {
		return types.FileContractID{}, nil, err
	}

	// Read the signed response from the renter.
	var challengeResponse crypto.Signature
	err = encoding.ReadObject(conn, &challengeResponse, len(challengeResponse))
	if err != nil {
		return types.FileContractID{}, nil, err
	}
	// Verify the response. In the process, fetch the related storage
	// obligation, file contract revision, and transaction signatures.
	so, recentRevision, revisionSigs, err := h.verifyChallengeResponse(fcid, challenge, challengeResponse)
	if err != nil {
		return types.FileContractID{}, nil, modules.WriteNegotiationRejection(conn, err)
	}

	// Send the file contract revision and the corresponding signatures to the
	// renter.
	err = modules.WriteNegotiationAcceptance(conn)
	if err != nil {
		return types.FileContractID{}, nil, err
	}
	err = encoding.WriteObject(conn, recentRevision)
	if err != nil {
		return types.FileContractID{}, nil, err
	}
	err = encoding.WriteObject(conn, revisionSigs)
	if err != nil {
		return types.FileContractID{}, nil, err
	}
	return fcid, so, nil
}
