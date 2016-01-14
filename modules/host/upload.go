package host

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// maxRevisionSize is the maximum number of bytes added in a single revision
	maxRevisionSize = 100e6 // 100 MB
)

var (
	// HostCapacityErr indicates that a host does not have enough room on disk
	// to accept more files.
	HostCapacityErr = errors.New("host is at capacity and cannot take more files")
)

// considerContract checks that the provided transaction matches the host's
// terms, and doesn't contain any flagrant errors.
func (h *Host) considerContract(txn types.Transaction, renterKey types.SiaPublicKey, filesize uint64, merkleRoot crypto.Hash) error {
	// Check that there is only one file contract.
	if len(txn.FileContracts) != 1 {
		return errors.New("transaction should have only one file contract")
	}

	// convenience variables
	fc := txn.FileContracts[0]
	duration := fc.WindowStart - h.blockHeight
	voidAddress := types.UnlockHash{}

	// check contract fields for sanity and acceptability
	switch {
	case fc.FileSize != filesize:
		return errors.New("bad initial file size")
	case fc.WindowStart <= h.blockHeight:
		return errors.New("window start cannot be in the past")
	case duration < h.settings.MinDuration || duration > h.settings.MaxDuration:
		return errors.New("duration is out of bounds")
	case fc.WindowEnd <= fc.WindowStart:
		return errors.New("window cannot end before it starts")
	case fc.WindowEnd-fc.WindowStart < h.settings.WindowSize:
		return errors.New("challenge window is not large enough")
	case fc.FileMerkleRoot != merkleRoot:
		return errors.New("bad file contract Merkle root")
	case fc.Payout.IsZero():
		return errors.New("bad file contract payout")
	case len(fc.ValidProofOutputs) != 2:
		return errors.New("bad file contract valid proof outputs")
	case len(fc.MissedProofOutputs) != 2:
		return errors.New("bad file contract missed proof outputs")
	case fc.ValidProofOutputs[1].UnlockHash != h.settings.UnlockHash:
		return errors.New("file contract valid proof output not sent to host")
	case fc.MissedProofOutputs[1].UnlockHash != voidAddress:
		return errors.New("file contract missed proof output not sent to void")
	}

	// check unlock hash
	uc := types.UnlockConditions{
		PublicKeys:         []types.SiaPublicKey{renterKey, h.publicKey},
		SignaturesRequired: 2,
	}
	if fc.UnlockHash != uc.UnlockHash() {
		return errors.New("bad file contract unlock hash")
	}

	return nil
}

// considerRevision checks that the provided file contract revision is still
// acceptable to the host.
func (h *Host) considerRevision(txn types.Transaction, obligation *contractObligation) error {
	// Check that there is only one revision.
	if len(txn.FileContractRevisions) != 1 {
		return errors.New("transaction should have only one revision")
	}

	// calculate minimum expected output value
	rev := txn.FileContractRevisions[0]
	duration := types.NewCurrency64(uint64(obligation.windowStart() - h.blockHeight))
	minHostPrice := types.NewCurrency64(rev.NewFileSize).Mul(duration).Mul(h.settings.Price)
	expectedPayout := types.PostTax(h.blockHeight, obligation.payout())

	switch {
	// these fields should never change
	case rev.ParentID != obligation.ID:
		return errors.New("bad revision parent ID")
	case rev.NewWindowStart != obligation.windowStart():
		return errors.New("bad revision window start")
	case rev.NewWindowEnd != obligation.windowEnd():
		return errors.New("bad revision window end")
	case rev.NewUnlockHash != obligation.unlockHash():
		return errors.New("bad revision unlock hash")
	case rev.UnlockConditions.UnlockHash() != obligation.unlockHash():
		return errors.New("bad revision unlock conditions")
	case len(rev.NewValidProofOutputs) != 2:
		return errors.New("bad revision valid proof outputs")
	case len(rev.NewMissedProofOutputs) != 2:
		return errors.New("bad revision missed proof outputs")
	case rev.NewValidProofOutputs[1].UnlockHash != obligation.validProofUnlockHash(),
		rev.NewMissedProofOutputs[1].UnlockHash != obligation.missedProofUnlockHash():
		return errors.New("bad revision proof outputs")

	case rev.NewRevisionNumber <= obligation.revisionNumber():
		return errors.New("revision must have higher revision number")

	case rev.NewFileSize > uint64(h.spaceRemaining):
		return errors.New("revision file size is too large")
	case rev.NewFileSize <= obligation.fileSize():
		return errors.New("revision must add data")
	case rev.NewFileSize-obligation.fileSize() > maxRevisionSize:
		return errors.New("revision adds too much data")

	case rev.NewValidProofOutputs[0].Value.Add(rev.NewValidProofOutputs[1].Value).Cmp(expectedPayout) != 0,
		// valid and missing outputs should still sum to payout
		rev.NewMissedProofOutputs[0].Value.Add(rev.NewMissedProofOutputs[1].Value).Cmp(expectedPayout) != 0:
		return errors.New("revision outputs do not sum to original payout")

	case rev.NewValidProofOutputs[1].Value.Cmp(minHostPrice) < 0:
		// outputs should have been adjusted proportional to the new filesize
		return errors.New("revision price is too small")

	case rev.NewMissedProofOutputs[0].Value.Cmp(rev.NewValidProofOutputs[0].Value) != 0:
		return errors.New("revision missed renter payout does not match valid payout")
	}

	return nil
}

// managedNegotiateContract negotiates a file contract with a renter, and adds
// the metadata to the host's obligation set. The filesize, merkleRoot, and
// filename arguments are provided to make managedNegotiateContract usable
// with both rpcUpload and rpcRenew.
func (h *Host) managedNegotiateContract(conn net.Conn, filesize uint64, merkleRoot crypto.Hash, filename string) error {
	// allow 5 minutes for contract negotiation
	err := conn.SetDeadline(time.Now().Add(5 * time.Minute))
	if err != nil {
		return err
	}

	// Exchange keys between the renter and the host.
	//
	// TODO: This is vulnerable to MITM attacks, the renter should be getting
	// the host's key from the blockchain.
	if err = encoding.WriteObject(conn, h.publicKey); err != nil {
		return errors.New("couldn't write our public key: " + err.Error())
	}
	var renterKey types.SiaPublicKey
	if err := encoding.ReadObject(conn, &renterKey, 256); err != nil {
		return errors.New("couldn't read the renter's public key: " + err.Error())
	}

	// Read the initial transaction set, which will contain a file contract and
	// any required parent transactions.
	var unsignedTxnSet []types.Transaction
	if err := encoding.ReadObject(conn, &unsignedTxnSet, maxContractLen); err != nil {
		return errors.New("couldn't read the initial transaction set: " + err.Error())
	}
	if len(unsignedTxnSet) == 0 {
		return errors.New("initial transaction set was empty")
	}

	// The transaction with the file contract should be the last transaction in
	// the set. Verify that the terms of the contract are favorable to the
	// host, then accept the contract.
	contractTxn := unsignedTxnSet[len(unsignedTxnSet)-1]
	h.mu.RLock()
	err = h.considerContract(contractTxn, renterKey, filesize, merkleRoot)
	h.mu.RUnlock()
	if err != nil {
		_ = encoding.WriteObject(conn, err.Error())
		return errors.New("rejected file contract: " + err.Error())
	}
	if err := encoding.WriteObject(conn, modules.AcceptResponse); err != nil {
		return errors.New("couldn't write acceptance: " + err.Error())
	}

	// Add collateral to the transaction and send the new transaction.
	// Currently, collateral is not supported, so the unchanged transaction set
	// is returned.
	if err := encoding.WriteObject(conn, unsignedTxnSet); err != nil {
		return errors.New("couldn't write collateral transaction set: " + err.Error())
	}

	// The renter will sign the transaction set, agreeing to pay the host.
	var signedTxnSet []types.Transaction
	if err := encoding.ReadObject(conn, &signedTxnSet, maxContractLen); err != nil {
		return errors.New("couldn't read signed transaction set:" + err.Error())
	}

	// The host will verify that the signed transaction set provided by the
	// renter is the same transaction set that the host considered previously.
	if len(signedTxnSet) != len(unsignedTxnSet) {
		return errors.New("renter sent bad signed transaction set")
	}
	for i := range signedTxnSet {
		if signedTxnSet[i].ID() != unsignedTxnSet[i].ID() {
			return errors.New("renter sent bad signed transaction set")
		}
	}

	// The host now signs the transaction set, confirming the collateral, and
	// then submits the transaction set to the blockchain.
	signedTxn, parents := signedTxnSet[len(signedTxnSet)-1], signedTxnSet[:len(signedTxnSet)-1]
	txnBuilder := h.wallet.RegisterTransaction(signedTxn, parents)
	signedTxnSet, err = txnBuilder.Sign(true)
	if err != nil {
		return err
	}
	err = h.tpool.AcceptTransactionSet(signedTxnSet)
	if err == modules.ErrDuplicateTransactionSet {
		// This can happen if the host is uploading to itself.
		err = nil
	}
	if err != nil {
		return err
	}

	// Add this contract to the host's list of obligations.
	co := &contractObligation{
		ID:                contractTxn.FileContractID(0),
		OriginTransaction: contractTxn,
		Path:              filename,
	}
	h.mu.Lock()
	h.addObligation(co)
	h.mu.Unlock()
	if err != nil {
		return err
	}

	// Send the fully signed and valid transaction set back to the renter.
	if err := encoding.WriteObject(conn, signedTxnSet); err != nil {
		return errors.New("couldn't write signed transaction set: " + err.Error())
	}

	return nil
}

// managedRPCUpload is an RPC that negotiates a file contract. Under the new
// scheme, file contracts should not initially hold any data.
func (h *Host) managedRPCUpload(conn net.Conn) error {
	// Check that the host has grabbed an address from the wallet.
	h.mu.Lock()
	acceptingContracts := h.acceptingContracts
	uh := h.settings.UnlockHash
	h.fileCounter++ // Harmless to increment the file counter in the event of an error.
	filename := filepath.Join(h.persistDir, strconv.Itoa(int(h.fileCounter)))
	h.mu.Unlock()

	// Check that the host is accepting new file contracts.
	if !acceptingContracts {
		// No error needs to be returned, because the host is functioning as
		// intended. The host should not be accepting file contracts, so the
		// function ends immediately.
		return nil
	}

	if uh == (types.UnlockHash{}) {
		return errors.New("couldn't negotiate contract: host does not have an address")
	}

	// negotiate expecting empty Merkle root
	return h.managedNegotiateContract(conn, 0, crypto.Hash{}, filename)
}

// managedRPCRevise is an RPC that allows a renter to revise a file contract. It will
// read new revisions in a loop until the renter sends a termination signal.
func (h *Host) managedRPCRevise(conn net.Conn) error {
	// Check that the host is accepting new file contracts.
	h.mu.RLock()
	ac := h.acceptingContracts
	h.mu.RUnlock()
	if !ac {
		// The host is not accepting revisions, but the host is not
		// experiencing an error, because it is intentionally still listening
		// for RPCs. Return nil without processing the request.
		//
		// Revisions are rejected because they increase the size of the file,
		// which may be undesirable to the operator.
		return nil
	}
	// read ID of contract to be revised
	var fcid types.FileContractID
	if err := encoding.ReadObject(conn, &fcid, crypto.HashSize); err != nil {
		return errors.New("couldn't read contract ID: " + err.Error())
	}

	// remove conn deadline while we wait for lock and rebuild the Merkle tree.
	err := conn.SetDeadline(time.Now().Add(15 * time.Minute))
	if err != nil {
		return err
	}

	h.mu.RLock()
	obligation, exists := h.obligationsByID[fcid]
	h.mu.RUnlock()
	if !exists {
		return errors.New("no record of that contract")
	}
	// need to protect against two simultaneous revisions to the same
	// contract; this can cause inconsistency and data loss, making storage
	// proofs impossible
	//
	// TODO: DOS vector - the host has locked the obligation even though the
	// renter has not proven themselves to be the owner of the file contract.
	obligation.mu.Lock()
	defer obligation.mu.Unlock()

	// open the file in append mode
	file, err := os.OpenFile(obligation.Path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
	if err != nil {
		return err
	}

	// rebuild current Merkle tree
	tree := crypto.NewTree()
	err = tree.ReadSegments(file)
	if err != nil {
		// Error does not need to be checked when closing the file, already
		// there have been issues related to the filesystem.
		_ = file.Close()
		return err
	}

	// accept new revisions in a loop. The final good transaction will be
	// submitted to the blockchain.
	revisionErr := func() error {
		for {
			// allow 5 minutes between revisions
			err := conn.SetDeadline(time.Now().Add(5 * time.Minute))
			if err != nil {
				return err
			}

			// read proposed revision
			var revTxn types.Transaction
			if err = encoding.ReadObject(conn, &revTxn, types.BlockSizeLimit); err != nil {
				return errors.New("couldn't read revision: " + err.Error())
			}
			// an empty transaction indicates completion
			if revTxn.ID() == (types.Transaction{}).ID() {
				return nil
			}

			// allow 5 minutes for each revision
			err = conn.SetDeadline(time.Now().Add(5 * time.Minute))
			if err != nil {
				return err
			}

			// check revision against original file contract
			h.mu.RLock()
			err = h.considerRevision(revTxn, obligation)
			h.mu.RUnlock()
			if err != nil {
				// There is nothing that can be done if there is an error while
				// writing to a connection.
				_ = encoding.WriteObject(conn, err.Error())
				return err
			}

			// indicate acceptance
			if err := encoding.WriteObject(conn, modules.AcceptResponse); err != nil {
				return errors.New("couldn't write acceptance: " + err.Error())
			}

			// read piece
			// TODO: simultaneously read into tree and file
			rev := revTxn.FileContractRevisions[0]
			piece := make([]byte, rev.NewFileSize-obligation.fileSize())
			_, err = io.ReadFull(conn, piece)
			if err != nil {
				return errors.New("couldn't read piece data: " + err.Error())
			}

			// verify Merkle root
			err = tree.ReadSegments(bytes.NewReader(piece))
			if err != nil {
				return errors.New("couldn't verify Merkle root: " + err.Error())
			}
			if tree.Root() != rev.NewFileMerkleRoot {
				return errors.New("revision has bad Merkle root")
			}

			// manually sign the transaction
			revTxn.TransactionSignatures = append(revTxn.TransactionSignatures, types.TransactionSignature{
				ParentID:       crypto.Hash(fcid),
				CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
				PublicKeyIndex: 1, // host key is always second
			})
			encodedSig, err := crypto.SignHash(revTxn.SigHash(1), h.secretKey)
			if err != nil {
				return err
			}
			revTxn.TransactionSignatures[1].Signature = encodedSig[:]

			// append piece to file
			if _, err := file.Write(piece); err != nil {
				return errors.New("couldn't write new data to file: " + err.Error())
			}

			// save updated obligation to disk
			h.mu.Lock()
			h.reviseObligation(revTxn)
			h.mu.Unlock()

			// send the signed transaction - this must be the last thing that happens.
			if err := encoding.WriteObject(conn, revTxn); err != nil {
				return errors.New("couldn't write signed revision transaction: " + err.Error())
			}
		}
	}()
	err = file.Close()
	if err != nil {
		return err
	}

	err = h.tpool.AcceptTransactionSet([]types.Transaction{obligation.RevisionTransaction})
	if err != nil {
		h.log.Println("WARN: transaction pool rejected revision transaction: " + err.Error())
	}
	return revisionErr
}

// managedRPCRenew is an RPC that allows a renter to renew a file contract. The
// protocol is identical to standard contract negotiation, except that the
// Merkle root is copied over from the old contract.
func (h *Host) managedRPCRenew(conn net.Conn) error {
	// Check that the host is accepting new file contracts.
	h.mu.RLock()
	ac := h.acceptingContracts
	h.mu.RUnlock()
	if !ac {
		// The host is not accepting new file contracts, but the host is not
		// experiencing an error, because it is intentionally still listening
		// for RPCs. Return nil without processing the request.
		return nil
	}

	// read ID of contract to be renewed
	var fcid types.FileContractID
	if err := encoding.ReadObject(conn, &fcid, crypto.HashSize); err != nil {
		return errors.New("couldn't read contract ID: " + err.Error())
	}

	h.mu.RLock()
	obligation, exists := h.obligationsByID[fcid]
	h.mu.RUnlock()
	if !exists {
		return errors.New("no record of that contract")
	}
	// need to protect against simultaneous renewals of the same contract
	obligation.mu.Lock()
	defer obligation.mu.Unlock()

	// copy over old file data
	h.mu.RLock()
	h.fileCounter++
	filename := filepath.Join(h.persistDir, strconv.Itoa(int(h.fileCounter)))
	h.mu.RUnlock()

	// TODO: What happens if the copy operation fails partway through? Does
	// there need to be garbage collection at startup for failed uploads that
	// might still be on disk?
	old, err := os.Open(obligation.Path)
	if err != nil {
		return err
	}
	renewed, err := os.Create(filename)
	if err != nil {
		return err
	}
	_, err = io.Copy(renewed, old)
	if err != nil {
		return err
	}

	return h.managedNegotiateContract(conn, obligation.fileSize(), obligation.merkleRoot(), filename)
}
