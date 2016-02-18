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

var (
	// the contractor will not form contracts above this price
	maxPrice = types.SiacoinPrecision.Div(types.NewCurrency64(4320e9)).Mul(types.NewCurrency64(500)) // 500 SC / GB / Month

	errTooExpensive = errors.New("host price was too high")
)

// negotiateContract establishes a connection to a host and negotiates an
// initial file contract according to the terms of the host.
func negotiateContract(conn net.Conn, addr modules.NetAddress, fc types.FileContract, txnBuilder transactionBuilder, tpool transactionPool) (Contract, error) {
	// allow 30 seconds for negotiation
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// read host key
	var hostPublicKey types.SiaPublicKey
	if err := encoding.ReadObject(conn, &hostPublicKey, 256); err != nil {
		return Contract{}, errors.New("couldn't read host's public key: " + err.Error())
	}

	// create our key
	ourSK, ourPK, err := crypto.GenerateKeyPair()
	if err != nil {
		return Contract{}, errors.New("failed to generate keypair: " + err.Error())
	}
	ourPublicKey := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       ourPK[:],
	}

	// send our public key
	if err := encoding.WriteObject(conn, ourPublicKey); err != nil {
		return Contract{}, errors.New("couldn't send our public key: " + err.Error())
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
		return Contract{}, err
	}
	txnBuilder.AddFileContract(fc)
	txn, parents := txnBuilder.View()
	txnSet := append(parents, txn)

	// calculate contract ID
	fcid := txn.FileContractID(0) // TODO: is it actually 0?

	// send txn
	if err := encoding.WriteObject(conn, txnSet); err != nil {
		return Contract{}, errors.New("couldn't send our proposed contract: " + err.Error())
	}

	// read back acceptance
	var response string
	if err := encoding.ReadObject(conn, &response, 128); err != nil {
		return Contract{}, errors.New("couldn't read the host's response to our proposed contract: " + err.Error())
	}
	if response != modules.AcceptResponse {
		return Contract{}, errors.New("host rejected proposed contract: " + response)
	}

	// read back txn with host collateral.
	var hostTxnSet []types.Transaction
	if err := encoding.ReadObject(conn, &hostTxnSet, types.BlockSizeLimit); err != nil {
		return Contract{}, errors.New("couldn't read the host's updated contract: " + err.Error())
	}

	// check that txn is okay. For now, no collateral will be added, so the
	// transaction sets should be identical.
	if len(hostTxnSet) != len(txnSet) {
		return Contract{}, errors.New("host sent bad collateral transaction")
	}
	for i := range hostTxnSet {
		if hostTxnSet[i].ID() != txnSet[i].ID() {
			return Contract{}, errors.New("host sent bad collateral transaction")
		}
	}

	// sign the txn and resend
	// NOTE: for now, we are assuming that the transaction has not changed
	// since we sent it. Otherwise, the txnBuilder would have to be updated
	// with whatever fields were added by the host.
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return Contract{}, err
	}
	if err := encoding.WriteObject(conn, signedTxnSet); err != nil {
		return Contract{}, errors.New("couldn't send the contract signed by us: " + err.Error())
	}

	// read signed txn from host
	var signedHostTxnSet []types.Transaction
	if err := encoding.ReadObject(conn, &signedHostTxnSet, types.BlockSizeLimit); err != nil {
		return Contract{}, errors.New("couldn't read the contract signed by the host: " + err.Error())
	}

	// submit to blockchain
	err = tpool.AcceptTransactionSet(signedHostTxnSet)
	if err == modules.ErrDuplicateTransactionSet {
		// this can happen if the renter is uploading to itself
		err = nil
	}
	if err != nil {
		return Contract{}, err
	}

	// create host contract
	contract := Contract{
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

	return contract, nil
}

// newContract negotiates an initial file contract with the specified host
// and returns a Contract. The contract is also saved by the HostDB.
func (c *Contractor) newContract(host modules.HostSettings, filesize uint64, duration types.BlockHeight) (Contract, error) {
	// reject hosts that are too expensive
	if host.Price.Cmp(maxPrice) > 0 {
		return Contract{}, errTooExpensive
	}

	// get an address to use for negotiation
	c.mu.Lock()
	if c.cachedAddress == (types.UnlockHash{}) {
		uc, err := c.wallet.NextAddress()
		if err != nil {
			c.mu.Unlock()
			return Contract{}, err
		}
		c.cachedAddress = uc.UnlockHash()
	}
	ourAddress := c.cachedAddress
	c.mu.Unlock()

	// create file contract
	renterCost := host.Price.Mul(types.NewCurrency64(filesize)).Mul(types.NewCurrency64(uint64(duration)))
	renterCost = renterCost.MulFloat(1.05) // extra buffer to guarantee we won't run out of money during revision
	payout := renterCost                   // no collateral

	c.mu.RLock()
	height := c.blockHeight
	c.mu.RUnlock()

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
	txnBuilder := c.wallet.StartTransaction()

	// initiate connection
	conn, err := c.dialer.DialTimeout(host.NetAddress, 15*time.Second)
	if err != nil {
		return Contract{}, err
	}
	defer conn.Close()
	if err := encoding.WriteObject(conn, modules.RPCUpload); err != nil {
		return Contract{}, err
	}

	// execute negotiation protocol
	contract, err := negotiateContract(conn, host.NetAddress, fc, txnBuilder, c.tpool)
	if err != nil {
		txnBuilder.Drop() // return unused outputs to wallet
		return Contract{}, err
	}

	c.mu.Lock()
	c.contracts[contract.ID] = contract
	// clear the cached address
	c.cachedAddress = types.UnlockHash{}
	c.save()
	c.mu.Unlock()

	return contract, nil
}
