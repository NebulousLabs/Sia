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

// managedSendRecentRevision sends the most recent known file contract
// revision, including signatures, to the renter, for the file contract with
// the input id.
func (h *Host) managedRPCRevisionRequest(conn net.Conn) (types.FileContractID, error) {
	// Set the negotiation deadline.
	conn.SetDeadline(time.Now().Add(modules.NegotiateRevisionRequestTime))

	// Receive the file contract id from the renter.
	var fcid types.FileContractID
	err := encoding.ReadObject(conn, &fcid, uint64(len(fcid)))
	if err != nil {
		return types.FileContractID{}, err
	}

	// Fetch the storage obligation with the file contract revision
	// transaction.
	var so *storageObligation
	err = h.db.Update(func(tx *bolt.Tx) error {
		fso, err := getStorageObligation(tx, fcid)
		so = &fso
		return err
	})
	if err != nil {
		return types.FileContractID{}, composeErrors(err, modules.WriteNegotiationRejection(conn, err))
	}

	// Send the most recent file contract revision.
	revisionTxn := so.RevisionTransactionSet[len(so.RevisionTransactionSet)-1]
	recentRevision := revisionTxn.FileContractRevisions[0]
	// Find all of the signatures on the file contract revision. There should be two.
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
		h.log.Critical("host is inconsistend, bad file contract revision transaction", err)
		return types.FileContractID{}, err
	}

	// Send the file contract revision and the corresponding signatures to the
	// renter.
	err = encoding.WriteObject(conn, revisionTxn)
	if err != nil {
		return types.FileContractID{}, err
	}
	err = encoding.WriteObject(conn, revisionSigs)
	if err != nil {
		return types.FileContractID{}, err
	}
	return fcid, nil
}
