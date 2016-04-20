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

// startRevision is run at the beginning of each revision iteration. It reads
// the host's settings confirms that the values are acceptable, and writes an acceptance.
func startRevision(conn net.Conn, host modules.HostDBEntry, hdb hostDB) error {
	// verify the host's settings and confirm its identity
	// TODO: return new host, so we can calculate price accurately
	_, err := verifySettings(conn, host, hdb)
	if err != nil {
		// TODO: doesn't make sense to reject here if the err is an I/O error.
		return modules.WriteNegotiationRejection(conn, err)
	}
	return modules.WriteNegotiationAcceptance(conn)
}

// verifyRecentRevision confirms that the host and contractor agree upon the current
// state of the contract being revisde.
func verifyRecentRevision(conn net.Conn, contract Contract) error {
	// send contract ID
	if err := encoding.WriteObject(conn, contract.ID); err != nil {
		return errors.New("couldn't send contract ID: " + err.Error())
	}
	// read challenge
	var challenge crypto.Hash
	if err := encoding.ReadObject(conn, &challenge, 32); err != nil {
		return errors.New("couldn't read challenge: " + err.Error())
	}
	// sign and return
	sig, err := crypto.SignHash(challenge, contract.SecretKey)
	if err != nil {
		return err
	} else if err := encoding.WriteObject(conn, sig); err != nil {
		return errors.New("couldn't send challenge response: " + err.Error())
	}
	// read acceptance
	if err := modules.ReadNegotiationAcceptance(conn); err != nil {
		return errors.New("host did not accept revision request: " + err.Error())
	}
	// read last revision and signatures
	var lastRevision types.FileContractRevision
	var hostSignatures []types.TransactionSignature
	if err := encoding.ReadObject(conn, &lastRevision, 2048); err != nil {
		return errors.New("couldn't read last revision: " + err.Error())
	}
	if err := encoding.ReadObject(conn, &hostSignatures, 2048); err != nil {
		return errors.New("couldn't read host signatures: " + err.Error())
	}
	// verify the revision and signatures
	// NOTE: we can fake the blockheight here because it doesn't affect
	// verification; it just needs to be above the fork height and below the
	// contract expiration (which was checked earlier).
	return modules.VerifyFileContractRevisionTransactionSignatures(lastRevision, hostSignatures, contract.FileContract.WindowStart-1)
}

// negotiateRevision sends the revision and new piece data to the host.
func negotiateRevision(conn net.Conn, rev types.FileContractRevision, secretKey crypto.SecretKey, blockheight types.BlockHeight) (types.Transaction, error) {
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
