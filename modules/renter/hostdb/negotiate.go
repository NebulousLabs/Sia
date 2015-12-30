package hostdb

import (
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var (
	// the hostdb will not form contracts above this price
	maxPrice = types.SiacoinPrecision.Div(types.NewCurrency64(4320e9)).Mul(types.NewCurrency64(500)) // 500 SC / GB / Month

	errTooExpensive = errors.New("host price was too high")
)

// negotiateContract establishes a connection to a host and negotiates an
// initial file contract according to the terms of the host.
func negotiateContract(conn net.Conn, addr modules.NetAddress, fc types.FileContract, txnBuilder hdbTransactionBuilder, tpool hdbTransactionPool) (hostContract, error) {
	// allow 30 seconds for negotiation
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// read host key
	var hostPublicKey types.SiaPublicKey
	if err := encoding.ReadObject(conn, &hostPublicKey, 256); err != nil {
		return hostContract{}, errors.New("couldn't read host's public key: " + err.Error())
	}

	// create our key
	ourSK, ourPK, err := crypto.GenerateKeyPair()
	if err != nil {
		return hostContract{}, errors.New("failed to generate keypair: " + err.Error())
	}
	ourPublicKey := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       ourPK[:],
	}

	// send our public key
	if err := encoding.WriteObject(conn, ourPublicKey); err != nil {
		return hostContract{}, errors.New("couldn't send our public key: " + err.Error())
	}

	// create unlock conditions
	uc := types.UnlockConditions{
		PublicKeys:         []types.SiaPublicKey{ourPublicKey, hostPublicKey},
		SignaturesRequired: 2,
	}

	// add UnlockHash to file contract
	fc.UnlockHash = uc.UnlockHash()

	// build transaction containing fc
	err = txnBuilder.FundSiacoins(fc.Payout)
	if err != nil {
		return hostContract{}, err
	}
	txnBuilder.AddFileContract(fc)
	txn, parents := txnBuilder.View()
	txnSet := append(parents, txn)

	// calculate contract ID
	fcid := txn.FileContractID(0) // TODO: is it actually 0?

	// send txn
	if err := encoding.WriteObject(conn, txnSet); err != nil {
		return hostContract{}, errors.New("couldn't send our proposed contract: " + err.Error())
	}

	// read back acceptance
	var response string
	if err := encoding.ReadObject(conn, &response, 128); err != nil {
		return hostContract{}, errors.New("couldn't read the host's response to our proposed contract: " + err.Error())
	}
	if response != modules.AcceptResponse {
		return hostContract{}, errors.New("host rejected proposed contract: " + response)
	}

	// read back txn with host collateral.
	var hostTxnSet []types.Transaction
	if err := encoding.ReadObject(conn, &hostTxnSet, types.BlockSizeLimit); err != nil {
		return hostContract{}, errors.New("couldn't read the host's updated contract: " + err.Error())
	}

	// check that txn is okay. For now, no collateral will be added, so the
	// transaction sets should be identical.
	if len(hostTxnSet) != len(txnSet) {
		return hostContract{}, errors.New("host sent bad collateral transaction")
	}
	for i := range hostTxnSet {
		if hostTxnSet[i].ID() != txnSet[i].ID() {
			return hostContract{}, errors.New("host sent bad collateral transaction")
		}
	}

	// sign the txn and resend
	// NOTE: for now, we are assuming that the transaction has not changed
	// since we sent it. Otherwise, the txnBuilder would have to be updated
	// with whatever fields were added by the host.
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return hostContract{}, err
	}
	if err := encoding.WriteObject(conn, signedTxnSet); err != nil {
		return hostContract{}, errors.New("couldn't send the contract signed by us: " + err.Error())
	}

	// read signed txn from host
	var signedHostTxnSet []types.Transaction
	if err := encoding.ReadObject(conn, &signedHostTxnSet, types.BlockSizeLimit); err != nil {
		return hostContract{}, errors.New("couldn't read the contract signed by the host: " + err.Error())
	}

	// submit to blockchain
	err = tpool.AcceptTransactionSet(signedHostTxnSet)
	if err == modules.ErrDuplicateTransactionSet {
		// this can happen if the renter is uploading to itself
		err = nil
	}
	if err != nil {
		return hostContract{}, err
	}

	// create host contract
	hc := hostContract{
		IP:           addr,
		ID:           fcid,
		FileContract: fc,
		LastRevision: types.FileContractRevision{
			ParentID:              fcid,
			UnlockConditions:      uc,
			NewRevisionNumber:     fc.RevisionNumber,
			NewFileSize:           fc.FileSize,
			NewFileMerkleRoot:     fc.FileMerkleRoot,
			NewWindowStart:        fc.WindowStart,
			NewWindowEnd:          fc.WindowEnd,
			NewValidProofOutputs:  []types.SiacoinOutput{fc.ValidProofOutputs[0], fc.ValidProofOutputs[1]},
			NewMissedProofOutputs: []types.SiacoinOutput{fc.MissedProofOutputs[0], fc.MissedProofOutputs[1]},
			NewUnlockHash:         fc.UnlockHash,
		},
		LastRevisionTxn: types.Transaction{},
		SecretKey:       ourSK,
	}

	return hc, nil
}

// newContract negotiates an initial file contract with the specified host
// and returns a hostContract. The contract is also saved by the HostDB.
func (hdb *HostDB) newContract(host modules.HostSettings, filesize uint64, duration types.BlockHeight) (hostContract, error) {
	// reject hosts that are too expensive
	if host.Price.Cmp(maxPrice) > 0 {
		return hostContract{}, errTooExpensive
	}

	// get an address to use for negotiation
	hdb.mu.Lock()
	if hdb.cachedAddress == (types.UnlockHash{}) {
		uc, err := hdb.wallet.NextAddress()
		if err != nil {
			hdb.mu.Unlock()
			return hostContract{}, err
		}
		hdb.cachedAddress = uc.UnlockHash()
	}
	ourAddress := hdb.cachedAddress
	hdb.mu.Unlock()

	// create file contract
	renterCost := host.Price.Mul(types.NewCurrency64(filesize)).Mul(types.NewCurrency64(uint64(duration)))
	renterCost = renterCost.MulFloat(1.05) // extra buffer to guarantee we won't run out of money during revision
	payout := renterCost                   // no collateral

	hdb.mu.RLock()
	height := hdb.blockHeight
	hdb.mu.RUnlock()

	fc := types.FileContract{
		FileSize:       0,
		FileMerkleRoot: crypto.Hash{}, // no proof possible without data
		WindowStart:    height + duration,
		WindowEnd:      height + duration + host.WindowSize,
		Payout:         payout,
		UnlockHash:     types.UnlockHash{}, // to be filled in by negotiateContract
		RevisionNumber: 0,
		ValidProofOutputs: []types.SiacoinOutput{
			// outputs need to account for tax
			{Value: types.PostTax(height, renterCost), UnlockHash: ourAddress},
			// no collateral
			{Value: types.ZeroCurrency, UnlockHash: host.UnlockHash},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// same as above
			{Value: types.PostTax(height, renterCost), UnlockHash: ourAddress},
			// goes to the void, not the renter
			{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
		},
	}

	// create transaction builder
	txnBuilder := hdb.wallet.StartTransaction()

	// initiate connection
	conn, err := net.DialTimeout("tcp", string(host.NetAddress), 15*time.Second)
	if err != nil {
		return hostContract{}, err
	}
	defer conn.Close()
	if err := encoding.WriteObject(conn, modules.RPCUpload); err != nil {
		return hostContract{}, err
	}

	// execute negotiation protocol
	contract, err := negotiateContract(conn, host.NetAddress, fc, txnBuilder, hdb.tpool)
	if err != nil {
		txnBuilder.Drop() // return unused outputs to wallet
		return hostContract{}, err
	}

	hdb.mu.Lock()
	hdb.contracts[contract.ID] = contract
	// clear the cached address
	hdb.cachedAddress = types.UnlockHash{}
	hdb.save()
	hdb.mu.Unlock()

	return contract, nil
}

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

// Renew negotiates a new contract for data already stored with a host. It
// returns the ID of the new contract. This is a blocking call that performs
// network I/O.
func (hdb *HostDB) Renew(fcid types.FileContractID, newEndHeight types.BlockHeight) (types.FileContractID, error) {
	hdb.mu.RLock()
	height := hdb.blockHeight
	hc, ok := hdb.contracts[fcid]
	host, eok := hdb.allHosts[hc.IP]
	hdb.mu.RUnlock()
	if !ok {
		return types.FileContractID{}, errors.New("no record of that contract")
	} else if !eok {
		return types.FileContractID{}, errors.New("no record of that host")
	} else if newEndHeight < height {
		return types.FileContractID{}, errors.New("cannot renew below current height")
	} else if host.Price.Cmp(maxPrice) > 0 {
		return types.FileContractID{}, errTooExpensive
	}

	// get an address to use for negotiation
	hdb.mu.Lock()
	if hdb.cachedAddress == (types.UnlockHash{}) {
		uc, err := hdb.wallet.NextAddress()
		if err != nil {
			hdb.mu.Unlock()
			return types.FileContractID{}, err
		}
		hdb.cachedAddress = uc.UnlockHash()
	}
	ourAddress := hdb.cachedAddress
	hdb.mu.Unlock()

	renterCost := host.Price.Mul(types.NewCurrency64(hc.LastRevision.NewFileSize)).Mul(types.NewCurrency64(uint64(newEndHeight - height)))
	renterCost = renterCost.MulFloat(1.05) // extra buffer to guarantee we won't run out of money during revision
	payout := renterCost                   // no collateral

	// create file contract
	fc := types.FileContract{
		FileSize:       hc.LastRevision.NewFileSize,
		FileMerkleRoot: hc.LastRevision.NewFileMerkleRoot,
		WindowStart:    newEndHeight,
		WindowEnd:      newEndHeight + host.WindowSize,
		Payout:         payout,
		UnlockHash:     types.UnlockHash{}, // to be filled in by negotiateContract
		RevisionNumber: 0,
		ValidProofOutputs: []types.SiacoinOutput{
			// nothing returned to us; everything goes to the host
			{Value: types.ZeroCurrency, UnlockHash: ourAddress},
			{Value: types.PostTax(height, renterCost), UnlockHash: host.UnlockHash},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// nothing returned to us; everything goes to the void
			{Value: types.ZeroCurrency, UnlockHash: ourAddress},
			{Value: types.PostTax(height, renterCost), UnlockHash: types.UnlockHash{}},
		},
	}

	// create transaction builder
	txnBuilder := hdb.wallet.StartTransaction()

	// initiate connection
	conn, err := net.DialTimeout("tcp", string(hc.IP), 15*time.Second)
	if err != nil {
		return types.FileContractID{}, err
	}
	defer conn.Close()
	if err := encoding.WriteObject(conn, modules.RPCRenew); err != nil {
		return types.FileContractID{}, errors.New("couldn't initiate RPC: " + err.Error())
	}
	if err := encoding.WriteObject(conn, fcid); err != nil {
		return types.FileContractID{}, errors.New("couldn't send contract ID: " + err.Error())
	}

	// execute negotiation protocol
	newContract, err := negotiateContract(conn, hc.IP, fc, txnBuilder, hdb.tpool)
	if err != nil {
		txnBuilder.Drop() // return unused outputs to wallet
		return types.FileContractID{}, err
	}

	// update host contract
	hdb.mu.Lock()
	hdb.contracts[newContract.ID] = newContract
	hdb.cachedAddress = types.UnlockHash{} // clear cachedAddress
	err = hdb.save()
	hdb.mu.Unlock()
	if err != nil {
		hdb.log.Println("WARN: failed to save the hostdb:", err)
	}

	return newContract.ID, nil
}
