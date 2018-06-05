package proto

import (
	"net"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/errors"
	"github.com/NebulousLabs/ratelimit"
)

// cachedMerkleRoot calculates the root of a set of existing Merkle roots.
func cachedMerkleRoot(roots []crypto.Hash) crypto.Hash {
	tree := crypto.NewCachedTree(sectorHeight) // NOTE: height is not strictly necessary here
	for _, h := range roots {
		tree.Push(h)
	}
	return tree.Root()
}

// A Editor modifies a Contract by calling the revise RPC on a host. It
// Editors are NOT thread-safe; calls to Upload must happen in serial.
type Editor struct {
	contractID  types.FileContractID
	contractSet *ContractSet
	conn        net.Conn
	closeChan   chan struct{}
	deps        modules.Dependencies
	hdb         hostDB
	host        modules.HostDBEntry
	once        sync.Once

	height types.BlockHeight
}

// shutdown terminates the revision loop and signals the goroutine spawned in
// NewEditor to return.
func (he *Editor) shutdown() {
	extendDeadline(he.conn, modules.NegotiateSettingsTime)
	// don't care about these errors
	_, _ = verifySettings(he.conn, he.host)
	_ = modules.WriteNegotiationStop(he.conn)
	close(he.closeChan)
}

// Close cleanly terminates the revision loop with the host and closes the
// connection.
func (he *Editor) Close() error {
	// using once ensures that Close is idempotent
	he.once.Do(he.shutdown)
	return he.conn.Close()
}

// Upload negotiates a revision that adds a sector to a file contract.
func (he *Editor) Upload(data []byte) (_ modules.RenterContract, _ crypto.Hash, err error) {
	// Acquire the contract.
	sc, haveContract := he.contractSet.Acquire(he.contractID)
	if !haveContract {
		return modules.RenterContract{}, crypto.Hash{}, errors.New("contract not present in contract set")
	}
	defer he.contractSet.Return(sc)
	contract := sc.header // for convenience

	// calculate price
	// TODO: height is never updated, so we'll wind up overpaying on long-running uploads
	blockBytes := types.NewCurrency64(modules.SectorSize * uint64(contract.LastRevision().NewWindowEnd-he.height))
	sectorStoragePrice := he.host.StoragePrice.Mul(blockBytes)
	sectorBandwidthPrice := he.host.UploadBandwidthPrice.Mul64(modules.SectorSize)
	sectorCollateral := he.host.Collateral.Mul(blockBytes)

	// to mitigate small errors (e.g. differing block heights), fudge the
	// price and collateral by 0.2%. This is only applied to hosts above
	// v1.0.1; older hosts use stricter math.
	if build.VersionCmp(he.host.Version, "1.0.1") > 0 {
		sectorStoragePrice = sectorStoragePrice.MulFloat(1 + hostPriceLeeway)
		sectorBandwidthPrice = sectorBandwidthPrice.MulFloat(1 + hostPriceLeeway)
		sectorCollateral = sectorCollateral.MulFloat(1 - hostPriceLeeway)
	}

	sectorPrice := sectorStoragePrice.Add(sectorBandwidthPrice)
	if contract.RenterFunds().Cmp(sectorPrice) < 0 {
		return modules.RenterContract{}, crypto.Hash{}, errors.New("contract has insufficient funds to support upload")
	}
	if contract.LastRevision().NewMissedProofOutputs[1].Value.Cmp(sectorCollateral) < 0 {
		return modules.RenterContract{}, crypto.Hash{}, errors.New("contract has insufficient collateral to support upload")
	}

	// calculate the new Merkle root
	sectorRoot := crypto.MerkleRoot(data)
	merkleRoot := sc.merkleRoots.checkNewRoot(sectorRoot)

	// create the action and revision
	actions := []modules.RevisionAction{{
		Type:        modules.ActionInsert,
		SectorIndex: uint64(sc.merkleRoots.len()),
		Data:        data,
	}}
	rev := newUploadRevision(contract.LastRevision(), merkleRoot, sectorPrice, sectorCollateral)

	// run the revision iteration
	defer func() {
		// Increase Successful/Failed interactions accordingly
		if err != nil {
			he.hdb.IncrementFailedInteractions(he.host.PublicKey)
			err = errors.Extend(err, modules.ErrHostFault)
		} else {
			he.hdb.IncrementSuccessfulInteractions(he.host.PublicKey)
		}

		// reset deadline
		extendDeadline(he.conn, time.Hour)
	}()

	// initiate revision
	extendDeadline(he.conn, modules.NegotiateSettingsTime)
	if err := startRevision(he.conn, he.host); err != nil {
		return modules.RenterContract{}, crypto.Hash{}, err
	}

	// record the change we are about to make to the contract. If we lose power
	// mid-revision, this allows us to restore either the pre-revision or
	// post-revision contract.
	walTxn, err := sc.recordUploadIntent(rev, sectorRoot, sectorStoragePrice, sectorBandwidthPrice)
	if err != nil {
		return modules.RenterContract{}, crypto.Hash{}, err
	}

	// send actions
	extendDeadline(he.conn, modules.NegotiateFileContractRevisionTime)
	if err := encoding.WriteObject(he.conn, actions); err != nil {
		return modules.RenterContract{}, crypto.Hash{}, err
	}

	// Disrupt here before sending the signed revision to the host.
	if he.deps.Disrupt("InterruptUploadBeforeSendingRevision") {
		return modules.RenterContract{}, crypto.Hash{},
			errors.New("InterruptUploadBeforeSendingRevision disrupt")
	}

	// send revision to host and exchange signatures
	extendDeadline(he.conn, connTimeout)
	signedTxn, err := negotiateRevision(he.conn, rev, contract.SecretKey)
	if err == modules.ErrStopResponse {
		// if host gracefully closed, close our connection as well; this will
		// cause the next operation to fail
		he.conn.Close()
	} else if err != nil {
		return modules.RenterContract{}, crypto.Hash{}, err
	}

	// Disrupt here before updating the contract.
	if he.deps.Disrupt("InterruptUploadAfterSendingRevision") {
		return modules.RenterContract{}, crypto.Hash{},
			errors.New("InterruptUploadAfterSendingRevision disrupt")
	}

	// update contract
	err = sc.commitUpload(walTxn, signedTxn, sectorRoot, sectorStoragePrice, sectorBandwidthPrice)
	if err != nil {
		return modules.RenterContract{}, crypto.Hash{}, err
	}

	return sc.Metadata(), sectorRoot, nil
}

// NewEditor initiates the contract revision process with a host, and returns
// an Editor.
func (cs *ContractSet) NewEditor(host modules.HostDBEntry, id types.FileContractID, currentHeight types.BlockHeight, hdb hostDB, cancel <-chan struct{}) (_ *Editor, err error) {
	sc, ok := cs.Acquire(id)
	if !ok {
		return nil, errors.New("invalid contract")
	}
	defer cs.Return(sc)
	contract := sc.header

	// Increase Successful/Failed interactions accordingly
	defer func() {
		// a revision mismatch is not necessarily the host's fault
		if err != nil && !IsRevisionMismatch(err) {
			hdb.IncrementFailedInteractions(contract.HostPublicKey())
			err = errors.Extend(err, modules.ErrHostFault)
		} else if err == nil {
			hdb.IncrementSuccessfulInteractions(contract.HostPublicKey())
		}
	}()

	conn, closeChan, err := initiateRevisionLoop(host, contract, modules.RPCReviseContract, cancel, cs.rl)
	if IsRevisionMismatch(err) && len(sc.unappliedTxns) > 0 {
		// we have desynced from the host. If we have unapplied updates from the
		// WAL, try applying them.
		conn, closeChan, err = initiateRevisionLoop(host, sc.unappliedHeader(), modules.RPCReviseContract, cancel, cs.rl)
		if err != nil {
			return nil, err
		}
		// applying the updates was successful; commit them to disk
		if err := sc.commitTxns(); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	// if we succeeded, we can safely discard the unappliedTxns
	for _, txn := range sc.unappliedTxns {
		txn.SignalUpdatesApplied()
	}
	sc.unappliedTxns = nil

	// the host is now ready to accept revisions
	return &Editor{
		host:        host,
		hdb:         hdb,
		height:      currentHeight,
		contractID:  id,
		contractSet: cs,
		conn:        conn,
		closeChan:   closeChan,
		deps:        cs.deps,
	}, nil
}

// initiateRevisionLoop initiates either the editor or downloader loop with
// host, depending on which rpc was passed.
func initiateRevisionLoop(host modules.HostDBEntry, contract contractHeader, rpc types.Specifier, cancel <-chan struct{}, rl *ratelimit.RateLimit) (net.Conn, chan struct{}, error) {
	c, err := (&net.Dialer{
		Cancel:  cancel,
		Timeout: 45 * time.Second, // TODO: Constant
	}).Dial("tcp", string(host.NetAddress))
	if err != nil {
		return nil, nil, err
	}
	conn := ratelimit.NewRLConn(c, rl, cancel)

	closeChan := make(chan struct{})
	go func() {
		select {
		case <-cancel:
			conn.Close()
		case <-closeChan:
		}
	}()

	// allot 2 minutes for RPC request + revision exchange
	extendDeadline(conn, modules.NegotiateRecentRevisionTime)
	defer extendDeadline(conn, time.Hour)
	if err := encoding.WriteObject(conn, rpc); err != nil {
		conn.Close()
		close(closeChan)
		return nil, closeChan, errors.New("couldn't initiate RPC: " + err.Error())
	}
	if err := verifyRecentRevision(conn, contract, host.Version); err != nil {
		conn.Close() // TODO: close gracefully if host has entered revision loop
		close(closeChan)
		return nil, closeChan, err
	}
	return conn, closeChan, nil
}
