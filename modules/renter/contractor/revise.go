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
func negotiateRevision(conn net.Conn, rev types.FileContractRevision, piece []byte, secretKey crypto.SecretKey) (types.Transaction, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Minute)) // sufficient to transfer 4 MB over 100 kbps
	defer conn.SetDeadline(time.Time{})               // reset timeout after each revision

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

	// transfer piece
	if _, err := conn.Write(piece); err != nil {
		return types.Transaction{}, errors.New("couldn't transfer piece: " + err.Error())
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
func newRevision(rev types.FileContractRevision, pieceLen uint64, merkleRoot crypto.Hash, piecePrice types.Currency) types.FileContractRevision {
	// prevent a negative currency panic
	if piecePrice.Cmp(rev.NewValidProofOutputs[0].Value) > 0 {
		// probably not enough money, but the host might accept it anyway
		piecePrice = rev.NewValidProofOutputs[0].Value
	}
	return types.FileContractRevision{
		ParentID:          rev.ParentID,
		UnlockConditions:  rev.UnlockConditions,
		NewRevisionNumber: rev.NewRevisionNumber + 1,
		NewFileSize:       rev.NewFileSize + pieceLen,
		NewFileMerkleRoot: merkleRoot,
		NewWindowStart:    rev.NewWindowStart,
		NewWindowEnd:      rev.NewWindowEnd,
		NewValidProofOutputs: []types.SiacoinOutput{
			// less returned to renter
			{Value: rev.NewValidProofOutputs[0].Value.Sub(piecePrice), UnlockHash: rev.NewValidProofOutputs[0].UnlockHash},
			// more given to host
			{Value: rev.NewValidProofOutputs[1].Value.Add(piecePrice), UnlockHash: rev.NewValidProofOutputs[1].UnlockHash},
		},
		NewMissedProofOutputs: []types.SiacoinOutput{
			// less returned to renter
			{Value: rev.NewMissedProofOutputs[0].Value.Sub(piecePrice), UnlockHash: rev.NewMissedProofOutputs[0].UnlockHash},
			// more given to void
			{Value: rev.NewMissedProofOutputs[1].Value.Add(piecePrice), UnlockHash: rev.NewMissedProofOutputs[1].UnlockHash},
		},
		NewUnlockHash: rev.NewUnlockHash,
	}
}
