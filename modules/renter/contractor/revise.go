package contractor

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// negotiateRevision sends a revision and actions to the host for approval,
// completing one iteration of the revision loop.
func negotiateRevision(conn net.Conn, rev types.FileContractRevision, actions []modules.RevisionAction, secretKey crypto.SecretKey, blockheight types.BlockHeight) (types.Transaction, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Minute)) // sufficient to transfer 4 MB over 100 kbps
	defer conn.SetDeadline(time.Now().Add(time.Hour)) // reset timeout after each revision

	// create transaction containing the revision
	signedTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{rev},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(rev.ParentID),
			CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
			PublicKeyIndex: 0, // renter key is always first -- see formContract
		}},
	}
	// sign the transaction
	encodedSig, _ := crypto.SignHash(signedTxn.SigHash(0), secretKey) // no error possible
	signedTxn.TransactionSignatures[0].Signature = encodedSig[:]

	// send the revision and actions
	if err := encoding.WriteObject(conn, rev); err != nil {
		return types.Transaction{}, errors.New("couldn't send revision: " + err.Error())
	}
	if err := encoding.WriteObject(conn, actions); err != nil {
		return types.Transaction{}, errors.New("couldn't send revision actions: " + err.Error())
	}

	// read acceptance
	if err := modules.ReadNegotiationAcceptance(conn); err != nil {
		return types.Transaction{}, errors.New("host did not accept revision: " + err.Error())
	}

	// send the new transaction signature
	if err := encoding.WriteObject(conn, signedTxn.TransactionSignatures[0]); err != nil {
		return types.Transaction{}, errors.New("couldn't send transaction signature: " + err.Error())
	}

	// read the host's acceptance and transaction signature
	if err := modules.ReadNegotiationAcceptance(conn); err != nil {
		return types.Transaction{}, errors.New("host did not accept revision: " + err.Error())
	}
	var hostSig types.TransactionSignature
	if err := encoding.ReadObject(conn, &hostSig, 16e3); err != nil {
		return types.Transaction{}, errors.New("couldn't read host's signature: " + err.Error())
	}
	// add the signature to the transaction and verify it
	signedTxn.TransactionSignatures = append(signedTxn.TransactionSignatures, hostSig)
	if err := signedTxn.StandaloneValid(blockheight); err != nil {
		return types.Transaction{}, err
	}
	return signedTxn, nil
}

// newRevision revises the current revision to cover a different number of
// sectors.
func newRevision(rev types.FileContractRevision, merkleRoot crypto.Hash, numSectors uint64, sectorPrice types.Currency) types.FileContractRevision {
	// move safely moves n coins from src to dest, avoiding negative currency
	// panics. The new values of src and dest are returned.
	move := func(n, src, dest types.Currency) (types.Currency, types.Currency) {
		if n.Cmp(src) > 0 {
			n = src
		}
		return src.Sub(n), dest.Add(n)
	}
	// calculate price difference
	var (
		valid0  = rev.NewValidProofOutputs[0].Value
		valid1  = rev.NewValidProofOutputs[1].Value
		missed0 = rev.NewMissedProofOutputs[0].Value
		missed1 = rev.NewMissedProofOutputs[1].Value
	)
	curSectors := rev.NewFileSize / modules.SectorSize
	if numSectors > curSectors {
		diffPrice := sectorPrice.Mul(types.NewCurrency64(numSectors - curSectors))
		// move valid payout from renter to host
		valid0, valid1 = move(diffPrice, valid0, valid1)
		// move missed payout from renter to void
		missed0, missed1 = move(diffPrice, missed0, missed1)
	} else if numSectors < curSectors {
		diffPrice := sectorPrice.Mul(types.NewCurrency64(curSectors - numSectors))
		// move valid payout from host to renter
		valid1, valid0 = move(diffPrice, valid1, valid0)
		// move missed payout from void to renter
		missed1, missed0 = move(diffPrice, missed1, missed0)
	}

	return types.FileContractRevision{
		ParentID:          rev.ParentID,
		UnlockConditions:  rev.UnlockConditions,
		NewRevisionNumber: rev.NewRevisionNumber + 1,
		NewFileSize:       numSectors * modules.SectorSize,
		NewFileMerkleRoot: merkleRoot,
		NewWindowStart:    rev.NewWindowStart,
		NewWindowEnd:      rev.NewWindowEnd,
		NewValidProofOutputs: []types.SiacoinOutput{
			{Value: valid0, UnlockHash: rev.NewValidProofOutputs[0].UnlockHash},
			{Value: valid1, UnlockHash: rev.NewValidProofOutputs[1].UnlockHash},
		},
		NewMissedProofOutputs: []types.SiacoinOutput{
			{Value: missed0, UnlockHash: rev.NewMissedProofOutputs[0].UnlockHash},
			{Value: missed1, UnlockHash: rev.NewMissedProofOutputs[1].UnlockHash},
			// TODO: void???
		},
		NewUnlockHash: rev.NewUnlockHash,
	}
}

// negotiateDownloadRevision sends a revision and actions to the host for
// approval.
func negotiateDownloadRevision(conn net.Conn, rev types.FileContractRevision, actions []modules.DownloadAction, secretKey crypto.SecretKey, blockheight types.BlockHeight) (types.Transaction, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Minute)) // sufficient to transfer 4 MB over 100 kbps
	defer conn.SetDeadline(time.Now().Add(time.Hour)) // reset timeout after each revision

	// create transaction containing the revision
	signedTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{rev},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(rev.ParentID),
			CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
			PublicKeyIndex: 0, // renter key is always first -- see formContract
		}},
	}
	// sign the transaction
	encodedSig, _ := crypto.SignHash(signedTxn.SigHash(0), secretKey) // no error possible
	signedTxn.TransactionSignatures[0].Signature = encodedSig[:]

	// send the revision and actions
	if err := encoding.WriteObject(conn, rev); err != nil {
		return types.Transaction{}, errors.New("couldn't send revision: " + err.Error())
	}
	if err := encoding.WriteObject(conn, actions); err != nil {
		return types.Transaction{}, errors.New("couldn't send download actions: " + err.Error())
	}

	// read acceptance
	if err := modules.ReadNegotiationAcceptance(conn); err != nil {
		return types.Transaction{}, errors.New("host did not accept revision: " + err.Error())
	}

	// send the new transaction signature
	if err := encoding.WriteObject(conn, signedTxn.TransactionSignatures[0]); err != nil {
		return types.Transaction{}, errors.New("couldn't send transaction signature: " + err.Error())
	}

	// read the host's acceptance and transaction signature
	if err := modules.ReadNegotiationAcceptance(conn); err != nil {
		return types.Transaction{}, errors.New("host did not accept revision: " + err.Error())
	}
	var hostSig types.TransactionSignature
	if err := encoding.ReadObject(conn, &hostSig, 16e3); err != nil {
		return types.Transaction{}, errors.New("couldn't read host's signature: " + err.Error())
	}
	// add the signature to the transaction and verify it
	signedTxn.TransactionSignatures = append(signedTxn.TransactionSignatures, hostSig)
	if err := signedTxn.StandaloneValid(blockheight); err != nil {
		// TODO: what should be done here? The host will still try to send
		// data, but we probably want to close the connection immediately and
		// punish the host somehow.
		return types.Transaction{}, err
	}
	return signedTxn, nil
}

// newDownloadRevision revises the current revision to cover the cost of
// downloading data.
func newDownloadRevision(rev types.FileContractRevision, downloadCost types.Currency) types.FileContractRevision {
	// move safely moves n coins from src to dest, avoiding negative currency
	// panics. The new values of src and dest are returned.
	move := func(n, src, dest types.Currency) (types.Currency, types.Currency) {
		if n.Cmp(src) > 0 {
			n = src
		}
		return src.Sub(n), dest.Add(n)
	}

	// move valid payout from renter to host
	valid0, valid1 := move(downloadCost, rev.NewValidProofOutputs[0].Value, rev.NewValidProofOutputs[1].Value)
	// move missed payout from renter to void
	missed0, missed1 := move(downloadCost, rev.NewMissedProofOutputs[0].Value, rev.NewMissedProofOutputs[1].Value)

	return types.FileContractRevision{
		ParentID:          rev.ParentID,
		UnlockConditions:  rev.UnlockConditions,
		NewRevisionNumber: rev.NewRevisionNumber + 1,
		NewFileSize:       rev.NewFileSize,
		NewFileMerkleRoot: rev.NewFileMerkleRoot,
		NewWindowStart:    rev.NewWindowStart,
		NewWindowEnd:      rev.NewWindowEnd,
		NewValidProofOutputs: []types.SiacoinOutput{
			{Value: valid0, UnlockHash: rev.NewValidProofOutputs[0].UnlockHash},
			{Value: valid1, UnlockHash: rev.NewValidProofOutputs[1].UnlockHash},
		},
		NewMissedProofOutputs: []types.SiacoinOutput{
			{Value: missed0, UnlockHash: rev.NewMissedProofOutputs[0].UnlockHash},
			{Value: missed1, UnlockHash: rev.NewMissedProofOutputs[1].UnlockHash},
			// TODO: void???
		},
		NewUnlockHash: rev.NewUnlockHash,
	}
}
