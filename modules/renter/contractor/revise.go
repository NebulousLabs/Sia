package contractor

import (
	"errors"
	"net"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// negotiateRevision sends a revision and actions to the host for approval,
// completing one iteration of the revision loop.
func negotiateRevision(conn net.Conn, rev types.FileContractRevision, secretKey crypto.SecretKey, blockheight types.BlockHeight) (types.Transaction, error) {
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

	// send the revision
	if err := encoding.WriteObject(conn, rev); err != nil {
		return types.Transaction{}, errors.New("couldn't send revision: " + err.Error())
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
		return types.Transaction{}, errors.New("host did not accept transaction signature: " + err.Error())
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
func newRevision(rev types.FileContractRevision, merkleRoot crypto.Hash, numSectors uint64, sectorPrice, sectorCollateral types.Currency) types.FileContractRevision {
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
		missed2 = rev.NewMissedProofOutputs[2].Value
	)
	curSectors := rev.NewFileSize / modules.SectorSize
	if numSectors > curSectors {
		diffPrice := sectorPrice.Mul64(numSectors - curSectors)
		diffCollateral := sectorCollateral.Mul64(numSectors - curSectors)
		// move valid payout from renter to host
		valid0, valid1 = move(diffPrice, valid0, valid1)
		// move missed payout from renter to void
		missed0, missed2 = move(diffPrice, missed0, missed2)
		// move missed collateral from host to void
		missed1, missed2 = move(diffCollateral, missed1, missed2)
	} else if numSectors < curSectors {
		diffPrice := sectorPrice.Mul64(curSectors - numSectors)
		diffCollateral := sectorCollateral.Mul64(curSectors - numSectors)
		// move valid payout from host to renter
		valid1, valid0 = move(diffPrice, valid1, valid0)
		// move missed payout from void to renter
		missed1, missed0 = move(diffPrice, missed1, missed0)
		// move missed collateral from void to host
		missed2, missed1 = move(diffCollateral, missed2, missed1)
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
			{Value: missed2, UnlockHash: rev.NewMissedProofOutputs[2].UnlockHash},
		},
		NewUnlockHash: rev.NewUnlockHash,
	}
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
	missed0, missed2 := move(downloadCost, rev.NewMissedProofOutputs[0].Value, rev.NewMissedProofOutputs[2].Value)

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
			rev.NewMissedProofOutputs[1], // host output is unchanged
			{Value: missed2, UnlockHash: rev.NewMissedProofOutputs[2].UnlockHash},
		},
		NewUnlockHash: rev.NewUnlockHash,
	}
}

// newModifyRevision revises the current revision to cover the cost of
// modifying sector data.
func newModifyRevision(rev types.FileContractRevision, merkleRoot crypto.Hash, uploadCost types.Currency) types.FileContractRevision {
	// move safely moves n coins from src to dest, avoiding negative currency
	// panics. The new values of src and dest are returned.
	move := func(n, src, dest types.Currency) (types.Currency, types.Currency) {
		if n.Cmp(src) > 0 {
			n = src
		}
		return src.Sub(n), dest.Add(n)
	}

	// move valid payout from renter to host
	valid0, valid1 := move(uploadCost, rev.NewValidProofOutputs[0].Value, rev.NewValidProofOutputs[1].Value)
	// move missed payout from renter to void
	missed0, missed2 := move(uploadCost, rev.NewMissedProofOutputs[0].Value, rev.NewMissedProofOutputs[2].Value)

	return types.FileContractRevision{
		ParentID:          rev.ParentID,
		UnlockConditions:  rev.UnlockConditions,
		NewRevisionNumber: rev.NewRevisionNumber + 1,
		NewFileSize:       rev.NewFileSize,
		NewFileMerkleRoot: merkleRoot,
		NewWindowStart:    rev.NewWindowStart,
		NewWindowEnd:      rev.NewWindowEnd,
		NewValidProofOutputs: []types.SiacoinOutput{
			{Value: valid0, UnlockHash: rev.NewValidProofOutputs[0].UnlockHash},
			{Value: valid1, UnlockHash: rev.NewValidProofOutputs[1].UnlockHash},
		},
		NewMissedProofOutputs: []types.SiacoinOutput{
			{Value: missed0, UnlockHash: rev.NewMissedProofOutputs[0].UnlockHash},
			rev.NewMissedProofOutputs[1], // host output is unchanged
			{Value: missed2, UnlockHash: rev.NewMissedProofOutputs[2].UnlockHash},
		},
		NewUnlockHash: rev.NewUnlockHash,
	}
}
