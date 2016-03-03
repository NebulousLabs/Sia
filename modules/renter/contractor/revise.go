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

// negotiateRevision sends the revision and new piece data to the host.
func negotiateRevision(conn net.Conn, rev types.FileContractRevision, secretKey crypto.SecretKey) (types.Transaction, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Minute)) // sufficient to transfer 4 MB over 100 kbps
	defer conn.SetDeadline(time.Now().Add(time.Hour)) // reset timeout after each revision

	// create transaction containing the revision
	signedTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{rev},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(rev.ParentID),
			CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
			PublicKeyIndex: 0, // renter key is always first -- see negotiateContract
		}},
	}
	// sign the transaction
	encodedSig, _ := crypto.SignHash(signedTxn.SigHash(0), secretKey) // no error possible
	signedTxn.TransactionSignatures[0].Signature = encodedSig[:]

	// send the transaction
	if err := encoding.WriteObject(conn, signedTxn); err != nil {
		return types.Transaction{}, errors.New("couldn't send revision transaction: " + err.Error())
	}

	// host sends acceptance
	var response string
	if err := encoding.ReadObject(conn, &response, 128); err != nil {
		return types.Transaction{}, errors.New("couldn't read host acceptance: " + err.Error())
	}
	if response != modules.AcceptResponse {
		return types.Transaction{}, errors.New("host rejected revision: " + response)
	}

	// read txn signed by host
	var signedHostTxn types.Transaction
	if err := encoding.ReadObject(conn, &signedHostTxn, types.BlockSizeLimit); err != nil {
		return types.Transaction{}, errors.New("couldn't read signed revision transaction: " + err.Error())
	}

	if signedHostTxn.ID() != signedTxn.ID() {
		return types.Transaction{}, errors.New("host sent bad signed transaction")
	}

	return signedHostTxn, nil
}

// newRevision revises the current revision to incorporate new data.
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
	curSectors := rev.NewFileSize / SectorSize
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
		NewFileSize:       numSectors * SectorSize,
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
		},
		NewUnlockHash: rev.NewUnlockHash,
	}
}
