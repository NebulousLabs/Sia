package renter

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

// A hostUploader uploads pieces to a host. It implements the uploader interface.
type hostUploader struct {
	// constants
	settings         modules.HostSettings
	masterKey        crypto.TwofishKey
	unlockConditions types.UnlockConditions
	secretKey        crypto.SecretKey

	// resources
	conn   net.Conn
	renter *Renter

	// these are updated after each revision
	contract fileContract
	tree     crypto.MerkleTree
	lastTxn  types.Transaction

	// revisions need to be serialized; if two threads are revising the same
	// contract at the same time, a race condition could cause inconsistency
	// and data loss.
	revisionLock sync.Mutex
}

func (hu *hostUploader) fileContract() fileContract {
	return hu.contract
}

func (hu *hostUploader) Close() error {
	// send an empty revision to indicate that we are finished
	encoding.WriteObject(hu.conn, types.Transaction{})
	hu.conn.Close()
	// submit the most recent revision to the blockchain
	err := hu.renter.tpool.AcceptTransactionSet([]types.Transaction{hu.lastTxn})
	if err != nil {
		hu.renter.log.Println("Could not submit final contract revision:", err)
	}
	return err
}

// negotiateContract establishes a connection to a host and negotiates an
// initial file contract according to the terms of the host.
func (hu *hostUploader) negotiateContract(filesize uint64, duration types.BlockHeight) error {
	conn, err := net.DialTimeout("tcp", string(hu.settings.IPAddress), 15*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	// inital calculations before connecting to host
	lockID := hu.renter.mu.RLock()
	height := hu.renter.blockHeight
	hu.renter.mu.RUnlock(lockID)

	renterCost := hu.settings.Price.Mul(types.NewCurrency64(filesize)).Mul(types.NewCurrency64(uint64(duration)))
	renterCost = renterCost.MulFloat(1.05) // extra buffer to guarantee we won't run out of money during revision
	payout := renterCost                   // no collateral

	// get an address from the wallet
	ourAddr, err := hu.renter.wallet.NextAddress()
	if err != nil {
		return err
	}

	// write rpcID
	if err := encoding.WriteObject(conn, modules.RPCUpload); err != nil {
		return errors.New("couldn't initiate RPC: " + err.Error())
	}

	// read host key
	// TODO: need to save this?
	var hostPublicKey types.SiaPublicKey
	if err := encoding.ReadObject(conn, &hostPublicKey, 256); err != nil {
		return errors.New("couldn't read host's public key: " + err.Error())
	}

	// create our own key by combining the renter entropy with the host key
	entropy := crypto.HashAll(hu.renter.entropy, hostPublicKey)
	ourSK, ourPK := crypto.StdKeyGen.GenerateDeterministic(entropy)
	ourPublicKey := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       ourPK[:],
	}
	hu.secretKey = ourSK // used to sign future revisions

	// send our public key
	if err := encoding.WriteObject(conn, ourPublicKey); err != nil {
		return errors.New("couldn't send our public key: " + err.Error())
	}

	// create unlock conditions
	hu.unlockConditions = types.UnlockConditions{
		PublicKeys:         []types.SiaPublicKey{ourPublicKey, hostPublicKey},
		SignaturesRequired: 2,
	}

	// create file contract
	fc := types.FileContract{
		FileSize:       0,
		FileMerkleRoot: crypto.Hash{}, // no proof possible without data
		WindowStart:    height + duration,
		WindowEnd:      height + duration + hu.settings.WindowSize,
		Payout:         payout,
		UnlockHash:     hu.unlockConditions.UnlockHash(),
		RevisionNumber: 0,
	}
	// outputs need account for tax
	fc.ValidProofOutputs = []types.SiacoinOutput{
		{Value: renterCost.Sub(types.Tax(hu.renter.blockHeight, fc.Payout)), UnlockHash: ourAddr.UnlockHash()},
		{Value: types.ZeroCurrency, UnlockHash: hu.settings.UnlockHash}, // no collateral
	}
	fc.MissedProofOutputs = []types.SiacoinOutput{
		// same as above
		fc.ValidProofOutputs[0],
		// goes to the void, not the renter
		{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
	}

	// build transaction containing fc
	txnBuilder := hu.renter.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(fc.Payout)
	if err != nil {
		return err
	}
	txnBuilder.AddFileContract(fc)
	txn, parents := txnBuilder.View()
	txnSet := append(parents, txn)

	// calculate contract ID
	fcid := txn.FileContractID(0) // TODO: is it actually 0?

	// send txn
	if err := encoding.WriteObject(conn, txnSet); err != nil {
		txnBuilder.Drop()
		return errors.New("couldn't send our proposed contract: " + err.Error())
	}

	// read back acceptance
	var response string
	if err := encoding.ReadObject(conn, &response, 128); err != nil {
		txnBuilder.Drop()
		return errors.New("couldn't read the host's response to our proposed contract: " + err.Error())
	}
	if response != modules.AcceptResponse {
		txnBuilder.Drop()
		return errors.New("host rejected proposed contract: " + response)
	}

	// read back txn with host collateral.
	var hostTxnSet []types.Transaction
	if err := encoding.ReadObject(conn, &hostTxnSet, types.BlockSizeLimit); err != nil {
		txnBuilder.Drop()
		return errors.New("couldn't read the host's updated contract: " + err.Error())
	}

	// check that txn is okay. For now, no collateral will be added, so the
	// transaction sets should be identical.
	if len(hostTxnSet) != len(txnSet) {
		txnBuilder.Drop()
		return errors.New("host sent bad collateral transaction")
	}
	for i := range hostTxnSet {
		if hostTxnSet[i].ID() != txnSet[i].ID() {
			txnBuilder.Drop()
			return errors.New("host sent bad collateral transaction")
		}
	}

	// sign the txn and resend
	// NOTE: for now, we are assuming that the transaction has not changed
	// since we sent it. Otherwise, the txnBuilder would have to be updated
	// with whatever fields were added by the host.
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		txnBuilder.Drop()
		return err
	}
	if err := encoding.WriteObject(conn, signedTxnSet); err != nil {
		txnBuilder.Drop()
		return errors.New("couldn't send the contract signed by us: " + err.Error())
	}

	// read signed txn from host
	var signedHostTxnSet []types.Transaction
	if err := encoding.ReadObject(conn, &signedHostTxnSet, types.BlockSizeLimit); err != nil {
		txnBuilder.Drop()
		return errors.New("couldn't read the contract signed by the host: " + err.Error())
	}

	// submit to blockchain
	err = hu.renter.tpool.AcceptTransactionSet(signedHostTxnSet)
	if err == modules.ErrDuplicateTransactionSet {
		// this can happen if the renter is uploading to itself
		err = nil
	}
	if err != nil {
		txnBuilder.Drop()
		return err
	}

	// create initial fileContract object
	hu.contract = fileContract{
		ID:          fcid,
		IP:          hu.settings.IPAddress,
		WindowStart: fc.WindowStart,
	}

	lockID = hu.renter.mu.Lock()
	hu.renter.contracts[fcid] = fc
	hu.renter.mu.Unlock(lockID)

	return nil
}

// addPiece revises an existing file contract with a host, and then uploads a
// piece to it.
func (hu *hostUploader) addPiece(p uploadPiece) error {
	// only one revision can happen at a time
	hu.revisionLock.Lock()
	defer hu.revisionLock.Unlock()

	// get old file contract from renter
	lockID := hu.renter.mu.RLock()
	fc, exists := hu.renter.contracts[hu.contract.ID]
	height := hu.renter.blockHeight
	hu.renter.mu.RUnlock(lockID)
	if !exists {
		return errors.New("no record of contract to revise")
	}

	// encrypt piece data
	key := deriveKey(hu.masterKey, p.chunkIndex, p.pieceIndex)
	encPiece, err := key.EncryptBytes(p.data)
	if err != nil {
		return err
	}

	// Revise the file contract. If revision fails, submit most recent
	// successful revision to the blockchain.
	err = hu.revise(fc, encPiece, height)
	if err != nil {
		hu.renter.tpool.AcceptTransactionSet([]types.Transaction{hu.lastTxn})
		return err
	}

	// update fileContract
	hu.contract.Pieces = append(hu.contract.Pieces, pieceData{
		Chunk:  p.chunkIndex,
		Piece:  p.pieceIndex,
		Offset: fc.FileSize, // end of old file
	})

	// update file contract in renter
	fc.RevisionNumber++
	fc.FileSize += uint64(len(encPiece))
	lockID = hu.renter.mu.Lock()
	hu.renter.contracts[hu.contract.ID] = fc
	hu.renter.save()
	hu.renter.mu.Unlock(lockID)

	return nil
}

func (hu *hostUploader) revise(fc types.FileContract, piece []byte, height types.BlockHeight) error {
	// calculate new merkle root
	r := bytes.NewReader(piece)
	buf := make([]byte, crypto.SegmentSize)
	for {
		_, err := io.ReadFull(r, buf)
		if err == io.EOF {
			break
		} else if err != nil && err != io.ErrUnexpectedEOF {
			return err
		}
		hu.tree.Push(buf)
	}

	// create revision
	rev := types.FileContractRevision{
		ParentID:          hu.contract.ID,
		UnlockConditions:  hu.unlockConditions,
		NewRevisionNumber: fc.RevisionNumber + 1,

		NewFileSize:           fc.FileSize + uint64(len(piece)),
		NewFileMerkleRoot:     hu.tree.Root(),
		NewWindowStart:        fc.WindowStart,
		NewWindowEnd:          fc.WindowEnd,
		NewValidProofOutputs:  fc.ValidProofOutputs,
		NewMissedProofOutputs: fc.MissedProofOutputs,
		NewUnlockHash:         fc.UnlockHash,
	}
	// transfer value of piece from renter to host
	safeDuration := uint64(fc.WindowStart - height + 20) // buffer in case host is behind
	piecePrice := types.NewCurrency64(uint64(len(piece))).Mul(types.NewCurrency64(safeDuration)).Mul(hu.settings.Price)
	// prevent a negative currency panic
	if piecePrice.Cmp(fc.ValidProofOutputs[0].Value) > 0 {
		// probably not enough money, but the host might accept it anyway
		piecePrice = fc.ValidProofOutputs[0].Value
	}
	rev.NewValidProofOutputs[0].Value = rev.NewValidProofOutputs[0].Value.Sub(piecePrice)   // less returned to renter
	rev.NewValidProofOutputs[1].Value = rev.NewValidProofOutputs[1].Value.Add(piecePrice)   // more given to host
	rev.NewMissedProofOutputs[0].Value = rev.NewMissedProofOutputs[0].Value.Sub(piecePrice) // less returned to renter
	rev.NewMissedProofOutputs[1].Value = rev.NewMissedProofOutputs[1].Value.Add(piecePrice) // more given to void

	// create transaction containing the revision
	signedTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{rev},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(hu.contract.ID),
			CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
			PublicKeyIndex: 0, // renter key is always first -- see negotiateContract
		}},
	}

	// sign the transaction
	encodedSig, err := crypto.SignHash(signedTxn.SigHash(0), hu.secretKey)
	if err != nil {
		return err
	}
	signedTxn.TransactionSignatures[0].Signature = encodedSig[:]

	// send the transaction
	if err := encoding.WriteObject(hu.conn, signedTxn); err != nil {
		return err
	}

	// host sends acceptance
	var response string
	if err := encoding.ReadObject(hu.conn, &response, 128); err != nil {
		return err
	}
	if response != modules.AcceptResponse {
		return errors.New("host rejected revision")
	}

	// transfer piece
	if _, err := hu.conn.Write(piece); err != nil {
		return err
	}

	// read txn signed by host
	var signedHostTxn types.Transaction
	if err := encoding.ReadObject(hu.conn, &signedHostTxn, types.BlockSizeLimit); err != nil {
		return err
	}
	if signedHostTxn.ID() != signedTxn.ID() {
		return errors.New("host sent bad signed transaction")
	} else if err = signedHostTxn.StandaloneValid(height); err != nil {
		return err
	}

	hu.lastTxn = signedHostTxn

	return nil
}

func (r *Renter) newHostUploader(settings modules.HostSettings, filesize uint64, duration types.BlockHeight, masterKey crypto.TwofishKey) (*hostUploader, error) {
	hu := &hostUploader{
		settings:  settings,
		masterKey: masterKey,
		tree:      crypto.NewTree(),
		renter:    r,
	}

	// TODO: maybe do this later?
	err := hu.negotiateContract(filesize, duration)
	if err != nil {
		return nil, err
	}

	// initiate the revision loop
	hu.conn, err = net.DialTimeout("tcp", string(hu.settings.IPAddress), 15*time.Second)
	if err != nil {
		return nil, err
	}
	if err := encoding.WriteObject(hu.conn, modules.RPCRevise); err != nil {
		return nil, err
	}
	if err := encoding.WriteObject(hu.conn, hu.contract.ID); err != nil {
		return nil, err
	}

	return hu, nil
}
