package renter

import (
	"errors"
	//"io"
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
	unlockConditions types.UnlockConditions // renter needs to save this!

	// resources
	conn   net.Conn
	renter *Renter

	// these are updated after each revision
	contract fileContract
	tree     crypto.MerkleTree

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
	encoding.WriteObject(hu.conn, types.FileContractRevision{})
	return hu.conn.Close()
}

// negotiateContract establishes a connection to a host and negotiates an
// initial file contract according to the terms of the host.
func (hu *hostUploader) negotiateContract(filesize uint64, duration types.BlockHeight) error {
	conn, err := net.DialTimeout("tcp", string(hu.settings.IPAddress), 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	// inital calculations before connecting to host
	lockID := hu.renter.mu.RLock()
	height := hu.renter.blockHeight
	hu.renter.mu.RUnlock(lockID)

	renterCost := hu.settings.Price.Mul(types.NewCurrency64(filesize)).Mul(types.NewCurrency64(uint64(duration)))
	payout := renterCost // no collateral

	// get an address from the wallet
	ourAddr, err := hu.renter.wallet.NextAddress()
	if err != nil {
		return err
	}

	// write rpcID
	if err := encoding.WriteObject(conn, modules.RPCUpload); err != nil {
		return err
	}

	// read host key
	// TODO: need to save this?
	var hostPublicKey types.SiaPublicKey
	if err := encoding.ReadObject(conn, &hostPublicKey, 256); err != nil {
		return err
	}

	// create our own key by combining the renter entropy with the host key
	entropy := crypto.HashAll(hu.renter.entropy, hostPublicKey)
	ourPK, _ := crypto.DeterministicSignatureKeys(entropy)
	ourPublicKey := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       ourPK[:],
	}

	// create unlock conditions
	uc := types.UnlockConditions{
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
		ValidProofOutputs: []types.SiacoinOutput{
			{Value: renterCost, UnlockHash: ourAddr.UnlockHash()},
			{Value: types.ZeroCurrency, UnlockHash: hu.settings.UnlockHash}, // no collateral
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// same as above
			{Value: renterCost, UnlockHash: ourAddr.UnlockHash()},
			// goes to the void, not the renter
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
		UnlockHash:     uc.UnlockHash(),
		RevisionNumber: 0,
	}

	// build transaction containing fc
	txnBuilder := hu.renter.wallet.StartTransaction()
	err = txnBuilder.FundSiacoins(renterCost)
	if err != nil {
		return err
	}
	txnBuilder.AddFileContract(fc)
	txn, _ := txnBuilder.View()

	// calculate contract ID
	fcid := txn.FileContractID(0) // TODO: is it actually 0?

	// send txn
	if err := encoding.WriteObject(conn, txn); err != nil {
		return err
	}

	// read back acceptance
	var response string
	if err := encoding.ReadObject(conn, &response, 128); err != nil {
		return err
	}
	if response != modules.AcceptResponse {
		return errors.New("host rejected terms")
	}

	// read back txn with host collateral.
	var hostTxn types.Transaction
	if err := encoding.ReadObject(conn, &hostTxn, types.BlockSizeLimit); err != nil {
		return err
	}

	// check that txn is okay. For now, no collateral will be added.
	// TODO: are additional checks needed?
	if hostTxn.ID() != txn.ID() {
		return errors.New("host sent bad collateral transaction")
	}

	// sign the txn and resend
	txnBuilder = hu.renter.wallet.RegisterTransaction(hostTxn, nil)
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return err
	}
	if err := encoding.WriteObject(conn, signedTxnSet[0]); err != nil {
		return err
	}

	// read signed txn from host
	var signedHostTxn types.Transaction
	if err := encoding.ReadObject(conn, &signedHostTxn, types.BlockSizeLimit); err != nil {
		return err
	}

	// check that the txn is the same
	if signedTxnSet[0].ID() != signedHostTxn.ID() {
		return errors.New("host sent bad signed transaction")
	} else if err = signedHostTxn.StandaloneValid(height); err != nil {
		return err
	}

	// submit to blockchain
	hu.renter.tpool.AcceptTransactionSet([]types.Transaction{signedHostTxn})

	// create initial fileContract object
	hu.contract = fileContract{
		ID:          fcid,
		IP:          hu.settings.IPAddress,
		WindowStart: fc.WindowStart,
	}

	return nil
}

// addPiece revises an existing file contract with a host, and then uploads a
// piece to it.
// TODO: if something goes wrong, we need to submit the current revision.
func (hu *hostUploader) addPiece(p uploadPiece) error {
	// only one revision can happen at a time
	hu.revisionLock.Lock()
	defer hu.revisionLock.Unlock()

	// encrypt piece data
	key := deriveKey(hu.masterKey, p.chunkIndex, p.pieceIndex)
	encPiece, err := key.EncryptBytes(p.data)
	if err != nil {
		return err
	}

	// calculate new merkle root
	hu.tree.Push(encPiece) // TODO: WRONG!

	// get old file contract from renter
	lockID := hu.renter.mu.RLock()
	fc, exists := hu.renter.contracts[hu.contract.ID]
	height := hu.renter.blockHeight
	hu.renter.mu.RUnlock(lockID)
	if !exists {
		return errors.New("no record of contract to revise")
	}

	// create revision
	rev := types.FileContractRevision{
		ParentID:          hu.contract.ID,
		UnlockConditions:  hu.unlockConditions,
		NewRevisionNumber: fc.RevisionNumber + 1,

		NewFileSize:           fc.FileSize + uint64(len(encPiece)),
		NewFileMerkleRoot:     hu.tree.Root(),
		NewWindowStart:        fc.WindowStart,
		NewWindowEnd:          fc.WindowEnd,
		NewValidProofOutputs:  fc.ValidProofOutputs,
		NewMissedProofOutputs: fc.MissedProofOutputs,
		NewUnlockHash:         fc.UnlockHash,
	}
	// transfer value of piece from renter to host
	safeDuration := uint64(fc.WindowStart - height + 20) // buffer in case host is behind
	piecePrice := types.NewCurrency64(uint64(len(encPiece))).Mul(types.NewCurrency64(safeDuration)).Mul(hu.settings.Price)
	rev.NewValidProofOutputs[0].Value = rev.NewValidProofOutputs[0].Value.Sub(piecePrice)   // less returned to renter
	rev.NewValidProofOutputs[1].Value = rev.NewValidProofOutputs[1].Value.Add(piecePrice)   // more given to host
	rev.NewMissedProofOutputs[0].Value = rev.NewMissedProofOutputs[0].Value.Sub(piecePrice) // less returned to renter
	rev.NewMissedProofOutputs[1].Value = rev.NewMissedProofOutputs[1].Value.Add(piecePrice) // more given to void

	// send revision
	if err := encoding.WriteObject(hu.conn, rev); err != nil {
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
	if _, err := hu.conn.Write(encPiece); err != nil {
		return err
	}

	// create, sign, and send transaction
	txnBuilder := hu.renter.wallet.StartTransaction()
	renterCost := rev.NewValidProofOutputs[0].Value
	err = txnBuilder.FundSiacoins(renterCost)
	if err != nil {
		return err
	}
	txnBuilder.AddFileContract(fc)
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return err
	}
	if err := encoding.WriteObject(hu.conn, signedTxnSet); err != nil {
		return err
	}

	// read txn signed by host
	var signedHostTxn types.Transaction
	if err := encoding.ReadObject(hu.conn, &signedHostTxn, types.BlockSizeLimit); err != nil {
		return err
	}
	if err = signedHostTxn.StandaloneValid(height); err != nil {
		return err
	}

	// update fileContract
	hu.contract.Pieces = append(hu.contract.Pieces, pieceData{
		Chunk:  p.chunkIndex,
		Piece:  p.pieceIndex,
		Offset: fc.FileSize, // end of old file
	})

	// update file contract in renter
	fc.RevisionNumber = rev.NewRevisionNumber
	fc.FileSize = rev.NewFileSize
	lockID = hu.renter.mu.Lock()
	hu.renter.contracts[hu.contract.ID] = fc
	hu.renter.save()
	hu.renter.mu.Unlock(lockID)

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
	hu.conn, err = net.DialTimeout("tcp", string(hu.settings.IPAddress), 5*time.Second)
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
