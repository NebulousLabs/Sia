package host

import (
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// fetchRevision verifies that a request for a revision can be managed by the
// host, and then returns that revision to the host.
func (h *Host) fetchRevision(fcid types.FileContractID) (*storageObligation, types.FileContractRevision, []types.TransactionSignature, error) {
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
		if sig.ParentID == crypto.Hash(fcid) {
			revisionSigs = append(revisionSigs, sig)
		}
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

	// Fetch the file contract revision.
	so, recentRevision, revisionSigs, err := h.fetchRevision(fcid)
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
