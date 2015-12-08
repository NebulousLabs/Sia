package hostdb

import (
	"bytes"
	"errors"
	"net"
	"sync"
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

// An Uploader uploads data to a host.
type Uploader interface {
	// Upload revises the underlying contract to store the new data. It
	// returns the offset of the data in the stored file.
	Upload(data []byte) (offset uint64, err error)

	// Address returns the address of the host.
	Address() modules.NetAddress

	// ContractID returns the FileContractID of the contract.
	ContractID() types.FileContractID

	// EndHeight returns the height at which the contract ends.
	EndHeight() types.BlockHeight

	// Close terminates the connection to the uploader.
	Close() error
}

// A hostUploader uploads pieces to a host. It implements the uploader interface.
type hostUploader struct {
	// constants
	settings         modules.HostSettings
	unlockConditions types.UnlockConditions
	secretKey        crypto.SecretKey
	fcid             types.FileContractID

	// resources
	conn net.Conn
	hdb  *HostDB

	// these are updated after each revision
	tree    crypto.MerkleTree
	lastTxn types.Transaction

	// revisions need to be serialized; if two threads are revising the same
	// contract at the same time, a race condition could cause inconsistency
	// and data loss.
	revisionLock sync.Mutex
}

func (hu *hostUploader) Address() modules.NetAddress {
	return hu.settings.IPAddress
}

func (hu *hostUploader) ContractID() types.FileContractID {
	hu.revisionLock.Lock()
	defer hu.revisionLock.Unlock()
	return hu.fcid
}

func (hu *hostUploader) EndHeight() types.BlockHeight {
	hu.revisionLock.Lock()
	defer hu.revisionLock.Unlock()
	return hu.lastTxn.FileContractRevisions[0].NewWindowStart
}

func (hu *hostUploader) Close() error {
	// send an empty revision to indicate that we are finished
	encoding.WriteObject(hu.conn, types.Transaction{})
	hu.conn.Close()
	// submit the most recent revision to the blockchain
	err := hu.hdb.tpool.AcceptTransactionSet([]types.Transaction{hu.lastTxn})
	if err != nil {
	}
	return err
}

// negotiateContract establishes a connection to a host and negotiates an
// initial file contract according to the terms of the host.
func (hu *hostUploader) negotiateContract(filesize uint64, duration types.BlockHeight, renterAddress types.UnlockHash) error {
	conn, err := net.DialTimeout("tcp", string(hu.settings.IPAddress), 15*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// inital calculations before connecting to host
	hu.hdb.mu.RLock()
	height := hu.hdb.blockHeight
	hu.hdb.mu.RUnlock()

	renterCost := hu.settings.Price.Mul(types.NewCurrency64(filesize)).Mul(types.NewCurrency64(uint64(duration)))
	renterCost = renterCost.MulFloat(1.05) // extra buffer to guarantee we won't run out of money during revision
	payout := renterCost                   // no collateral

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

	// create our key
	ourSK, ourPK, err := crypto.StdKeyGen.Generate()
	if err != nil {
		return errors.New("failed to generate keypair: " + err.Error())
	}
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
		{Value: renterCost.Sub(types.Tax(hu.hdb.blockHeight, fc.Payout)), UnlockHash: renterAddress},
		{Value: types.ZeroCurrency, UnlockHash: hu.settings.UnlockHash}, // no collateral
	}
	fc.MissedProofOutputs = []types.SiacoinOutput{
		// same as above
		fc.ValidProofOutputs[0],
		// goes to the void, not the renter
		{Value: types.ZeroCurrency, UnlockHash: types.UnlockHash{}},
	}

	// build transaction containing fc
	txnBuilder := hu.hdb.wallet.StartTransaction()
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
	err = hu.hdb.tpool.AcceptTransactionSet(signedHostTxnSet)
	if err == modules.ErrDuplicateTransactionSet {
		// this can happen if the renter is uploading to itself
		err = nil
	}
	if err != nil {
		txnBuilder.Drop()
		return err
	}

	hu.fcid = fcid
	// create initial revision
	hu.lastTxn.FileContractRevisions = []types.FileContractRevision{{
		ParentID:              fcid,
		UnlockConditions:      hu.unlockConditions,
		NewRevisionNumber:     fc.RevisionNumber,
		NewFileSize:           fc.FileSize,
		NewFileMerkleRoot:     fc.FileMerkleRoot,
		NewWindowStart:        fc.WindowStart,
		NewWindowEnd:          fc.WindowEnd,
		NewValidProofOutputs:  []types.SiacoinOutput{fc.ValidProofOutputs[0], fc.ValidProofOutputs[1]},
		NewMissedProofOutputs: []types.SiacoinOutput{fc.MissedProofOutputs[0], fc.MissedProofOutputs[1]},
		NewUnlockHash:         fc.UnlockHash,
	}}

	hu.hdb.mu.Lock()
	hu.hdb.contracts[fcid] = hostContract{
		ID:           fcid,
		FileContract: fc,
		LastRevisionTxn: types.Transaction{
			// first revision is empty
			FileContractRevisions: []types.FileContractRevision{{}},
		},
		SecretKey: ourSK,
	}
	hu.hdb.save()
	hu.hdb.mu.Unlock()

	return nil
}

// Upload revises an existing file contract with a host, and then uploads a
// piece to it.
func (hu *hostUploader) Upload(data []byte) (uint64, error) {
	// only one revision can happen at a time
	hu.revisionLock.Lock()
	defer hu.revisionLock.Unlock()

	// get old file contract from renter
	hu.hdb.mu.RLock()
	fc, exists := hu.hdb.contracts[hu.fcid]
	height := hu.hdb.blockHeight
	hu.hdb.mu.RUnlock()
	if !exists {
		return 0, errors.New("no record of contract to revise")
	}

	// offset is old filesize
	offset := hu.lastTxn.FileContractRevisions[0].NewFileSize

	// revise the file contract
	err := hu.revise(hu.lastTxn.FileContractRevisions[0], data, height)
	if err != nil {
		return 0, err
	}

	// update file contract in renter
	hu.hdb.mu.Lock()
	hu.hdb.contracts[hu.fcid] = fc
	hu.hdb.save()
	hu.hdb.mu.Unlock()

	return offset, nil
}

// revise revises the previous revision to cover piece and uploads both the
// revision and the piece data to the host.
func (hu *hostUploader) revise(rev types.FileContractRevision, piece []byte, height types.BlockHeight) error {
	hu.conn.SetDeadline(time.Now().Add(5 * time.Minute)) // sufficient to transfer 4 MB over 100 kbps
	defer hu.conn.SetDeadline(time.Time{})               // reset timeout after each revision

	// calculate new merkle root
	err := hu.tree.ReadSegments(bytes.NewReader(piece))
	if err != nil {
		return err
	}

	// calculate piece price
	safeDuration := uint64(rev.NewWindowStart - height + 20) // buffer in case host is behind
	piecePrice := types.NewCurrency64(uint64(len(piece))).Mul(types.NewCurrency64(safeDuration)).Mul(hu.settings.Price)
	// prevent a negative currency panic
	if piecePrice.Cmp(rev.NewValidProofOutputs[0].Value) > 0 {
		// probably not enough money, but the host might accept it anyway
		piecePrice = rev.NewValidProofOutputs[0].Value
	}

	// modify revision
	rev.NewRevisionNumber = rev.NewRevisionNumber + 1
	rev.NewFileSize = rev.NewFileSize + uint64(len(piece))
	rev.NewFileMerkleRoot = hu.tree.Root()
	rev.NewValidProofOutputs[0].Value = rev.NewValidProofOutputs[0].Value.Sub(piecePrice)   // less returned to renter
	rev.NewValidProofOutputs[1].Value = rev.NewValidProofOutputs[1].Value.Add(piecePrice)   // more given to host
	rev.NewMissedProofOutputs[0].Value = rev.NewMissedProofOutputs[0].Value.Sub(piecePrice) // less returned to renter
	rev.NewMissedProofOutputs[1].Value = rev.NewMissedProofOutputs[1].Value.Add(piecePrice) // more given to void

	// create transaction containing the revision
	signedTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{rev},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(hu.fcid),
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
		return errors.New("could not send revision transaction: " + err.Error())
	}

	// host sends acceptance
	var response string
	if err := encoding.ReadObject(hu.conn, &response, 128); err != nil {
		return errors.New("could not read host acceptance: " + err.Error())
	}
	if response != modules.AcceptResponse {
		return errors.New("host rejected revision: " + response)
	}

	// transfer piece
	if _, err := hu.conn.Write(piece); err != nil {
		return errors.New("could not transfer piece: " + err.Error())
	}

	// read txn signed by host
	var signedHostTxn types.Transaction
	if err := encoding.ReadObject(hu.conn, &signedHostTxn, types.BlockSizeLimit); err != nil {
		return errors.New("could not read signed revision transaction: " + err.Error())
	}

	if signedHostTxn.ID() != signedTxn.ID() {
		return errors.New("host sent bad signed transaction")
	} else if err = signedHostTxn.StandaloneValid(height); err != nil {
		return err
	}

	hu.lastTxn = signedHostTxn

	return nil
}

// newHostUploader negotiates an initial file contract with the specified host
// and returns a hostUploader, which satisfies the Uploader interface.
func (hdb *HostDB) newHostUploader(settings modules.HostSettings) (*hostUploader, error) {
	// reject hosts that are too expensive
	if settings.Price.Cmp(maxPrice) > 0 {
		return nil, errTooExpensive
	}

	hu := &hostUploader{
		settings: settings,
		tree:     crypto.NewTree(),
		hdb:      hdb,
	}

	// get an address to use for negotiation
	// TODO: use more than one shared address
	if hdb.cachedAddress == (types.UnlockHash{}) {
		uc, err := hdb.wallet.NextAddress()
		if err != nil {
			return nil, err
		}
		hdb.cachedAddress = uc.UnlockHash()
	}

	const filesize = 1e9  // 1 GB
	const duration = 4320 // 30 days

	err := hu.negotiateContract(filesize, duration, hdb.cachedAddress)
	if err != nil {
		return nil, err
	}

	// if negotiation was sucessful, clear the cached address
	hdb.cachedAddress = types.UnlockHash{}

	// initiate the revision loop
	hu.conn, err = net.DialTimeout("tcp", string(hu.settings.IPAddress), 15*time.Second)
	if err != nil {
		return nil, err
	}
	if err := encoding.WriteObject(hu.conn, modules.RPCRevise); err != nil {
		return nil, err
	}
	if err := encoding.WriteObject(hu.conn, hu.fcid); err != nil {
		return nil, err
	}

	return hu, nil
}

// A HostPool is a collection of hosts used to upload a file.
type HostPool interface {
	// UniqueHosts will return up to 'n' unique hosts that are not in 'old'.
	UniqueHosts(n int, old []modules.NetAddress) []Uploader

	// Close terminates all connections in the host pool.
	Close() error
}

// A pool is a collection of hostUploaders that satisfies the HostPool
// interface. New hosts are drawn from a HostDB, and contracts are negotiated
// with them on demand.
type pool struct {
	hosts []*hostUploader
	hdb   *HostDB
}

// Close closes all of the pool's open host connections, and submits their
// respective contract revisions to the transaction pool.
func (p *pool) Close() error {
	for _, h := range p.hosts {
		h.Close()
	}
	return nil
}

// UniqueHosts will return up to 'n' unique hosts that are not in 'exclude'.
// The pool draws from its set of active connections first, and then negotiates
// new contracts if more hosts are required. Note that this latter case
// requires network I/O, so the caller should always assume that UniqueHosts
// will block.
func (p *pool) UniqueHosts(n int, exclude []modules.NetAddress) (hosts []Uploader) {
	if n == 0 {
		return
	}

	// first reuse existing connections
outer:
	for _, h := range p.hosts {
		for _, ip := range exclude {
			if h.Address() == ip {
				continue outer
			}
		}
		hosts = append(hosts, h)
		if len(hosts) >= n {
			return hosts
		}
	}

	// form new contracts from randomly-picked nodes
	p.hdb.mu.Lock()
	randHosts := p.hdb.randomHosts(n*2, exclude)
	p.hdb.mu.Unlock()
	for _, host := range randHosts {
		hu, err := p.hdb.newHostUploader(host)
		if err != nil {
			continue
		}
		hosts = append(hosts, hu)
		p.hosts = append(p.hosts, hu)
		if len(hosts) >= n {
			break
		}
	}
	return hosts
}

// NewPool returns an empty HostPool, unless the HostDB contains no hosts at
// all.
func (hdb *HostDB) NewPool() (HostPool, error) {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()
	if hdb.isEmpty() {
		return nil, errors.New("HostDB is empty")
	}
	return &pool{hdb: hdb}, nil
}
