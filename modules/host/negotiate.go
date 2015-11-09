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
	HostCapacityErr = errors.New("host is at capacity and cannot take more files")
)

// deallocate deletes a file and restores its allocated space.
func (h *Host) deallocate(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	h.spaceRemaining += stat.Size()
	return os.Remove(path)
}

// considerContract checks that the provided transaction matches the host's
// terms, and doesn't contain any flagrant errors.
func (h *Host) considerContract(txn types.Transaction, renterKey types.SiaPublicKey) error {
	// Check that there is only one file contract.
	// TODO: check that the txn is empty except for the contract?
	if len(txn.FileContracts) != 1 {
		return errors.New("transaction should have only one file contract")
	}
	// convenience variables
	fc := txn.FileContracts[0]
	duration := fc.WindowStart - h.blockHeight
	voidAddress := types.UnlockHash{}

	// check contract fields for sanity and acceptability
	switch {
	case fc.FileSize != 0:
		return errors.New("initial file size must be 0")

	case fc.WindowStart <= h.blockHeight:
		return errors.New("window start cannot be in the past")

	case duration < h.MinDuration || duration > h.MaxDuration:
		return errors.New("duration is out of bounds")

	case fc.WindowEnd <= fc.WindowStart:
		return errors.New("window cannot end before it starts")

	case fc.WindowEnd-fc.WindowStart < h.WindowSize:
		return errors.New("challenge window is not large enough")

	case fc.FileMerkleRoot != crypto.Hash{}:
		return errors.New("bad file contract Merkle root")

	case fc.Payout.IsZero():
		return errors.New("bad file contract payout")

	case len(fc.ValidProofOutputs) != 2:
		return errors.New("bad file contract valid proof outputs")

	case len(fc.MissedProofOutputs) != 2:
		return errors.New("bad file contract missed proof outputs")

	case !fc.ValidProofOutputs[1].Value.IsZero(), !fc.MissedProofOutputs[1].Value.IsZero():
		return errors.New("file contract collateral is not zero")

	case fc.ValidProofOutputs[1].UnlockHash != h.UnlockHash:
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
// TODO: should take a txn and check that is only contains the single revision
func (h *Host) considerRevision(txn types.Transaction, obligation contractObligation) error {
	// Check that there is only one revision.
	// TODO: check that the txn is empty except for the revision?
	if len(txn.FileContractRevisions) != 1 {
		return errors.New("transaction should have only one revision")
	}
	// Check that we have a previous revision
	if len(obligation.LastRevisionTxn.FileContractRevisions) != 1 {
		return errors.New("can't revise without a previous revision")
	}

	// calculate minimum expected output value
	rev := txn.FileContractRevisions[0]
	lastRev := obligation.LastRevisionTxn.FileContractRevisions[0]
	fc := obligation.FileContract
	duration := types.NewCurrency64(uint64(fc.WindowStart - h.blockHeight))
	minHostPrice := types.NewCurrency64(rev.NewFileSize).Mul(duration).Mul(h.Price)
	expectedPayout := types.PostTax(h.blockHeight, fc.Payout)

	switch {
	// these fields should never change
	case rev.ParentID != obligation.ID:
		return errors.New("bad revision parent ID")
	case rev.NewWindowStart != fc.WindowStart:
		return errors.New("bad revision window start")
	case rev.NewWindowEnd != fc.WindowEnd:
		return errors.New("bad revision window end")
	case rev.NewUnlockHash != fc.UnlockHash:
		return errors.New("bad revision unlock hash")
	case rev.UnlockConditions.UnlockHash() != fc.UnlockHash:
		return errors.New("bad revision unlock conditions")
	case len(rev.NewValidProofOutputs) != 2:
		return errors.New("bad revision valid proof outputs")
	case len(rev.NewMissedProofOutputs) != 2:
		return errors.New("bad revision missed proof outputs")
	case rev.NewValidProofOutputs[1].UnlockHash != fc.ValidProofOutputs[1].UnlockHash,
		rev.NewMissedProofOutputs[1].UnlockHash != fc.MissedProofOutputs[1].UnlockHash:
		return errors.New("bad revision proof outputs")

	case rev.NewRevisionNumber <= lastRev.NewRevisionNumber:
		return errors.New("revision must have higher revision number")

	case rev.NewFileSize > uint64(h.spaceRemaining) || rev.NewFileSize > h.MaxFilesize:
		return errors.New("revision file size is too large")
	case rev.NewFileSize <= lastRev.NewFileSize:
		return errors.New("revision must add data")
	case rev.NewFileSize-lastRev.NewFileSize > maxRevisionSize:
		return errors.New("revision adds too much data")

	// valid and missing outputs should still sum to payout
	case rev.NewValidProofOutputs[0].Value.Add(rev.NewValidProofOutputs[1].Value).Cmp(expectedPayout) != 0,
		rev.NewMissedProofOutputs[0].Value.Add(rev.NewMissedProofOutputs[1].Value).Cmp(expectedPayout) != 0:
		return errors.New("revision outputs do not sum to original payout")

	// outputs should have been adjusted proportional to the new filesize
	case rev.NewValidProofOutputs[1].Value.Cmp(minHostPrice) <= 0:
		return errors.New("revision price is too small")
	case rev.NewMissedProofOutputs[0].Value.Cmp(rev.NewValidProofOutputs[0].Value) != 0:
		return errors.New("revision missed renter payout does not match valid payout")
	}

	return nil
}

// rpcUpload is an RPC that negotiates a file contract. Under the new scheme,
// file contracts should not initially hold any data.
func (h *Host) rpcUpload(conn net.Conn) error {
	// Check that the host has grabbed an address from the wallet.
	if h.UnlockHash == (types.UnlockHash{}) {
		return errors.New("host needs an address; have you properly announced?")
	}

	// allow 1 minute for contract negotiation
	conn.SetDeadline(time.Now().Add(1 * time.Minute))

	// perform key exchange
	if err := encoding.WriteObject(conn, h.publicKey); err != nil {
		return errors.New("couldn't write our public key: " + err.Error())
	}
	var renterKey types.SiaPublicKey
	if err := encoding.ReadObject(conn, &renterKey, 256); err != nil {
		return errors.New("couldn't read the renter's public key: " + err.Error())
	}

	// read initial transaction set
	var unsignedTxnSet []types.Transaction
	if err := encoding.ReadObject(conn, &unsignedTxnSet, maxContractLen); err != nil {
		return errors.New("couldn't read the initial transaction set: " + err.Error())
	}

	// check the contract transaction, which should be the last txn in the set.
	contractTxn := unsignedTxnSet[len(unsignedTxnSet)-1]
	h.mu.RLock()
	err := h.considerContract(contractTxn, renterKey)
	h.mu.RUnlock()
	if err != nil {
		encoding.WriteObject(conn, err.Error())
		return errors.New("rejected file contract: " + err.Error())
	}

	// send acceptance
	if err := encoding.WriteObject(conn, modules.AcceptResponse); err != nil {
		return errors.New("couldn't write acceptance: " + err.Error())
	}

	// add collateral to txn and send. For now, we never add collateral, so no
	// changes are made.
	if err := encoding.WriteObject(conn, unsignedTxnSet); err != nil {
		return errors.New("couldn't write collateral transaction set: " + err.Error())
	}

	// read signed transaction set
	var signedTxnSet []types.Transaction
	if err := encoding.ReadObject(conn, &signedTxnSet, maxContractLen); err != nil {
		return errors.New("couldn't read signed transaction set:" + err.Error())
	}

	// check that transaction set was not modified
	if len(signedTxnSet) != len(unsignedTxnSet) {
		return errors.New("renter sent bad signed transaction set")
	}
	for i := range signedTxnSet {
		if signedTxnSet[i].ID() != unsignedTxnSet[i].ID() {
			return errors.New("renter sent bad signed transaction set")
		}
	}

	// sign and submit to blockchain
	signedTxn, parents := signedTxnSet[len(signedTxnSet)-1], signedTxnSet[:len(signedTxnSet)-1]
	txnBuilder := h.wallet.RegisterTransaction(signedTxn, parents)
	signedTxnSet, err = txnBuilder.Sign(true)
	if err != nil {
		return err
	}
	err = h.tpool.AcceptTransactionSet(signedTxnSet)
	if err == modules.ErrDuplicateTransactionSet {
		// this can happen if the host is uploading to itself
		//
		// TODO: is it possible for renter to cause a collision, overwriting a
		// previous file contract?
		err = nil
	}
	if err != nil {
		return err
	}

	// send doubly-signed transaction set
	if err := encoding.WriteObject(conn, signedTxnSet); err != nil {
		return errors.New("couldn't write signed transaction set: " + err.Error())
	}

	// Add this contract to the host's list of obligations.
	// TODO: is there a race condition here?
	h.mu.Lock()
	h.fileCounter++
	co := &contractObligation{
		ID:           contractTxn.FileContractID(0),
		FileContract: contractTxn.FileContracts[0],
		Path:         filepath.Join(h.persistDir, strconv.Itoa(h.fileCounter)),
	}
	// first revision is empty
	co.LastRevisionTxn.FileContractRevisions = []types.FileContractRevision{{}}
	proofHeight := co.FileContract.WindowStart + StorageProofReorgDepth
	h.obligationsByHeight[proofHeight] = append(h.obligationsByHeight[proofHeight], co)
	h.obligationsByID[co.ID] = co
	h.save()
	h.mu.Unlock()

	return nil
}

// rpcRevise is an RPC that allows a renter to revise a file contract. It will
// read new revisions in a loop until the renter sends a termination signal.
func (h *Host) rpcRevise(conn net.Conn) error {
	// read ID of contract to be revised
	var fcid types.FileContractID
	if err := encoding.ReadObject(conn, &fcid, crypto.HashSize); err != nil {
		return errors.New("couldn't read contract ID: " + err.Error())
	}

	// remove conn deadline while we wait for lock and rebuild the Merkle tree
	conn.SetDeadline(time.Time{})

	h.mu.RLock()
	obligation, exists := h.obligationsByID[fcid]
	h.mu.RUnlock()
	if !exists {
		return errors.New("no record of that contract")
	}

	// need to protect against two simultaneous revisions to the same
	// contract; this can cause inconsistency and data loss, making storage
	// proofs impossible
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
		file.Close()
		return err
	}

	// accept new revisions in a loop. The final good transaction will be
	// submitted to the blockchain.
	revisionErr := func() error {
		for {
			// allow 2 minutes between revisions
			conn.SetDeadline(time.Now().Add(2 * time.Minute))

			// read proposed revision
			var revTxn types.Transaction
			if err := encoding.ReadObject(conn, &revTxn, types.BlockSizeLimit); err != nil {
				return errors.New("couldn't read revision: " + err.Error())
			}
			// an empty transaction indicates completion
			if revTxn.ID() == (types.Transaction{}).ID() {
				return nil
			}

			// allow 5 minutes for each revision
			conn.SetDeadline(time.Now().Add(5 * time.Minute))

			// check revision against original file contract
			h.mu.RLock()
			err := h.considerRevision(revTxn, *obligation)
			h.mu.RUnlock()
			if err != nil {
				encoding.WriteObject(conn, err.Error())
				continue // don't terminate loop; subsequent revisions may be okay
			}

			// indicate acceptance
			if err := encoding.WriteObject(conn, modules.AcceptResponse); err != nil {
				return errors.New("couldn't write acceptance: " + err.Error())
			}

			// read piece
			// TODO: simultaneously read into tree and file
			rev := revTxn.FileContractRevisions[0]
			last := obligation.LastRevisionTxn.FileContractRevisions[0]
			piece := make([]byte, rev.NewFileSize-last.NewFileSize)
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

			// send the signed transaction
			if err := encoding.WriteObject(conn, revTxn); err != nil {
				return errors.New("couldn't write signed revision transaction: " + err.Error())
			}

			// append piece to file
			if _, err := file.Write(piece); err != nil {
				return errors.New("couldn't write new data to file: " + err.Error())
			}

			// save updated obligation to disk
			h.mu.Lock()
			obligation.LastRevisionTxn = revTxn
			h.spaceRemaining -= int64(len(piece))
			h.save()
			h.mu.Unlock()
		}
	}()
	file.Close()

	// if a newly-created file was not updated, remove it
	if obligation.LastRevisionTxn.FileContractRevisions[0].NewRevisionNumber == 0 {
		os.Remove(obligation.Path)
		return revisionErr
	}

	err = h.tpool.AcceptTransactionSet([]types.Transaction{obligation.LastRevisionTxn})
	if err != nil {
		h.log.Println("WARN: transaction pool rejected revision transaction: " + err.Error())
	}

	return revisionErr
}
