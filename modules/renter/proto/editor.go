package proto

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

var hostPriceLeeway = build.Select(build.Var{
	Dev:      0.05,
	Standard: 0.002,
	Testing:  0.002,
}).(float64)

var (
	// sectorHeight is the height of a Merkle tree that covers a single
	// sector. It is log2(modules.SectorSize / crypto.SegmentSize)
	sectorHeight = func() uint64 {
		height := uint64(0)
		for 1<<height < (modules.SectorSize / crypto.SegmentSize) {
			height++
		}
		return height
	}()
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
	once        sync.Once
	host        modules.HostDBEntry
	hdb         hostDB

	height types.BlockHeight

	SaveFn revisionSaver
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

// runRevisionIteration submits actions and their accompanying revision to the
// host for approval. If negotiation is successful, it updates the underlying
// Contract.
func (he *Editor) runRevisionIteration(actions []modules.RevisionAction, contract modules.RenterContract, rev types.FileContractRevision, newRoots []crypto.Hash) (_ modules.RenterContract, err error) {
	defer func() {
		// Increase Successful/Failed interactions accordingly
		if err != nil {
			he.hdb.IncrementFailedInteractions(he.host.PublicKey)
		} else {
			he.hdb.IncrementSuccessfulInteractions(he.host.PublicKey)
		}

		// reset deadline
		extendDeadline(he.conn, time.Hour)
	}()

	// initiate revision
	extendDeadline(he.conn, modules.NegotiateSettingsTime)
	if err := startRevision(he.conn, he.host); err != nil {
		return modules.RenterContract{}, err
	}

	// Before we continue, save the revision. Unexpected termination (e.g.
	// power failure) during the signature transfer leaves in an ambiguous
	// state: the host may or may not have received the signature, and thus
	// may report either revision as being the most recent. To mitigate this,
	// we save the old revision as a fallback.
	if he.SaveFn != nil {
		if err := he.SaveFn(rev, newRoots); err != nil {
			return modules.RenterContract{}, err
		}
	}

	// send actions
	extendDeadline(he.conn, modules.NegotiateFileContractRevisionTime)
	if err := encoding.WriteObject(he.conn, actions); err != nil {
		return modules.RenterContract{}, err
	}

	// send revision to host and exchange signatures
	extendDeadline(he.conn, 2*time.Minute)
	signedTxn, err := negotiateRevision(he.conn, rev, contract.SecretKey)
	if err == modules.ErrStopResponse {
		// if host gracefully closed, close our connection as well; this will
		// cause the next operation to fail
		he.conn.Close()
	} else if err != nil {
		return modules.RenterContract{}, err
	}

	// update host contract
	contract.LastRevision = rev
	contract.LastRevisionTxn = signedTxn
	contract.MerkleRoots = newRoots
	return contract, nil
}

// Upload negotiates a revision that adds a sector to a file contract.
func (he *Editor) Upload(data []byte) (modules.RenterContract, crypto.Hash, error) {
	// Acquire the contract.
	contract, haveContract := he.contractSet.Acquire(he.contractID)
	if !haveContract {
		return modules.RenterContract{}, crypto.Hash{}, errors.New("contract not present in contract set")
	}
	defer func() { he.contractSet.Return(contract) }()

	// calculate price
	// TODO: height is never updated, so we'll wind up overpaying on long-running uploads
	blockBytes := types.NewCurrency64(modules.SectorSize * uint64(contract.FileContract.WindowEnd-he.height))
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
	if contract.LastRevision.NewMissedProofOutputs[1].Value.Cmp(sectorCollateral) < 0 {
		return modules.RenterContract{}, crypto.Hash{}, errors.New("contract has insufficient collateral to support upload")
	}

	// calculate the new Merkle root
	sectorRoot := crypto.MerkleRoot(data)
	newRoots := append(contract.MerkleRoots, sectorRoot)
	merkleRoot := cachedMerkleRoot(newRoots)

	// create the action and revision
	actions := []modules.RevisionAction{{
		Type:        modules.ActionInsert,
		SectorIndex: uint64(len(contract.MerkleRoots)),
		Data:        data,
	}}
	rev := newUploadRevision(contract.LastRevision, merkleRoot, sectorPrice, sectorCollateral)

	// run the revision iteration
	newContract, err := he.runRevisionIteration(actions, contract, rev, newRoots)
	if err != nil {
		return modules.RenterContract{}, crypto.Hash{}, err
	}
	contract = newContract

	// update metrics
	contract.StorageSpending = contract.StorageSpending.Add(sectorStoragePrice)
	contract.UploadSpending = contract.UploadSpending.Add(sectorBandwidthPrice)
	return contract, sectorRoot, nil
}

// Delete negotiates a revision that removes a sector from a file contract.
func (he *Editor) Delete(root crypto.Hash) (modules.RenterContract, error) {
	return modules.RenterContract{}, errors.New("not supported")
}

// Modify negotiates a revision that edits a sector in a file contract.
func (he *Editor) Modify(oldRoot, newRoot crypto.Hash, offset uint64, newData []byte) (modules.RenterContract, error) {
	return modules.RenterContract{}, errors.New("not supported")
}

// NewEditor initiates the contract revision process with a host, and returns
// an Editor.
func NewEditor(host modules.HostDBEntry, id types.FileContractID, contractSet *ContractSet, currentHeight types.BlockHeight, hdb hostDB, cancel <-chan struct{}) (_ *Editor, err error) {
	contract, ok := contractSet.View(id)
	// check that contract has enough value to support an upload
	if !ok || len(contract.LastRevision.NewValidProofOutputs) != 2 {
		return nil, errors.New("invalid contract")
	}

	// Increase Successful/Failed interactions accordingly
	defer func() {
		// a revision mismatch is not necessarily the host's fault
		if err != nil && !IsRevisionMismatch(err) {
			hdb.IncrementFailedInteractions(contract.HostPublicKey)
		} else if err == nil {
			hdb.IncrementSuccessfulInteractions(contract.HostPublicKey)
		}
	}()

	// initiate revision loop
	conn, err := (&net.Dialer{
		Cancel:  cancel,
		Timeout: 15 * time.Second,
	}).Dial("tcp", string(host.NetAddress))
	if err != nil {
		return nil, err
	}

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
	if err := encoding.WriteObject(conn, modules.RPCReviseContract); err != nil {
		conn.Close()
		close(closeChan)
		return nil, errors.New("couldn't initiate RPC: " + err.Error())
	}
	if err := verifyRecentRevision(conn, contract, host.Version); err != nil {
		conn.Close() // TODO: close gracefully if host has entered revision loop
		close(closeChan)
		return nil, err
	}

	// the host is now ready to accept revisions
	return &Editor{
		host:        host,
		hdb:         hdb,
		height:      currentHeight,
		contractID:  id,
		contractSet: contractSet,
		conn:        conn,
		closeChan:   closeChan,
	}, nil
}
