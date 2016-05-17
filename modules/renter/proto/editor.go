package proto

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
	conn net.Conn
	host modules.HostDBEntry

	height   types.BlockHeight
	contract Contract // updated after each revision

	// metrics
	StorageSpending types.Currency
	UploadSpending  types.Currency
}

// Close cleanly terminates the revision loop with the host and closes the
// connection.
func (he *Editor) Close() error {
	// don't care about these errors
	_, _ = verifySettings(he.conn, he.host)
	_ = modules.WriteNegotiationStop(he.conn)
	return he.conn.Close()
}

// runRevisionIteration submits actions and their accompanying revision to the
// host for approval. If negotiation is successful, it updates the underlying
// Contract.
func (he *Editor) runRevisionIteration(actions []modules.RevisionAction, rev types.FileContractRevision, newRoots []crypto.Hash) error {
	// initiate revision
	if err := startRevision(he.conn, he.host); err != nil {
		return err
	}

	// send actions
	if err := encoding.WriteObject(he.conn, actions); err != nil {
		return err
	}

	// send revision to host and exchange signatures
	signedTxn, err := negotiateRevision(he.conn, rev, he.contract.SecretKey)
	if err != nil {
		return err
	}

	// update host contract
	he.contract.LastRevision = rev
	he.contract.LastRevisionTxn = signedTxn
	he.contract.MerkleRoots = newRoots

	return nil
}

// Upload negotiates a revision that adds a sector to a file contract.
func (he *Editor) Upload(data []byte) (Contract, crypto.Hash, error) {
	// allot 10 minutes for this exchange; sufficient to transfer 4 MB over 50 kbps
	extendDeadline(he.conn, modules.NegotiateFileContractRevisionTime)
	defer extendDeadline(he.conn, time.Hour) // reset deadline

	// calculate price
	// TODO: height is never updated, so we'll wind up overpaying on long-running uploads
	blockBytes := types.NewCurrency64(modules.SectorSize * uint64(he.contract.FileContract.WindowEnd-he.height))
	sectorStoragePrice := he.host.StoragePrice.Mul(blockBytes)
	sectorBandwidthPrice := he.host.UploadBandwidthPrice.Mul64(modules.SectorSize)
	sectorPrice := sectorStoragePrice.Add(sectorBandwidthPrice)
	if sectorPrice.Cmp(he.contract.LastRevision.NewValidProofOutputs[0].Value) >= 0 {
		return Contract{}, crypto.Hash{}, errors.New("contract has insufficient funds to support upload")
	}
	sectorCollateral := he.host.Collateral.Mul(blockBytes)

	// calculate the new Merkle root
	sectorRoot := crypto.MerkleRoot(data)
	newRoots := append(he.contract.MerkleRoots, sectorRoot)
	merkleRoot := cachedMerkleRoot(newRoots)

	// create the action and revision
	actions := []modules.RevisionAction{{
		Type:        modules.ActionInsert,
		SectorIndex: uint64(len(he.contract.MerkleRoots)),
		Data:        data,
	}}
	rev := newRevision(he.contract.LastRevision, merkleRoot, uint64(len(newRoots)), sectorPrice, sectorCollateral)

	// run the revision iteration
	if err := he.runRevisionIteration(actions, rev, newRoots); err != nil {
		return Contract{}, crypto.Hash{}, err
	}

	// update metrics
	he.StorageSpending = he.StorageSpending.Add(sectorStoragePrice)
	he.UploadSpending = he.UploadSpending.Add(sectorBandwidthPrice)

	return he.contract, sectorRoot, nil
}

// Delete negotiates a revision that removes a sector from a file contract.
func (he *Editor) Delete(root crypto.Hash) (Contract, error) {
	// allot 2 minutes for this exchange
	extendDeadline(he.conn, 120*time.Second)
	defer extendDeadline(he.conn, time.Hour) // reset deadline

	// calculate price
	sectorPrice, sectorCollateral := types.ZeroCurrency, types.ZeroCurrency

	// calculate the new Merkle root
	newRoots := make([]crypto.Hash, 0, len(he.contract.MerkleRoots))
	index := -1
	for i, h := range he.contract.MerkleRoots {
		if h == root {
			index = i
		} else {
			newRoots = append(newRoots, h)
		}
	}
	if index == -1 {
		return Contract{}, errors.New("no record of that sector root")
	}
	merkleRoot := cachedMerkleRoot(newRoots)

	// create the action and accompanying revision
	actions := []modules.RevisionAction{{
		Type:        modules.ActionDelete,
		SectorIndex: uint64(index),
	}}
	rev := newRevision(he.contract.LastRevision, merkleRoot, uint64(len(newRoots)), sectorPrice, sectorCollateral)

	// run the revision iteration
	if err := he.runRevisionIteration(actions, rev, newRoots); err != nil {
		return Contract{}, err
	}
	return he.contract, nil
}

// Modify negotiates a revision that edits a sector in a file contract.
func (he *Editor) Modify(oldRoot, newRoot crypto.Hash, offset uint64, newData []byte) (Contract, error) {
	// allot 10 minutes for this exchange; sufficient to transfer 4 MB over 50 kbps
	extendDeadline(he.conn, modules.NegotiateFileContractRevisionTime)
	defer extendDeadline(he.conn, time.Hour) // reset deadline

	// calculate price
	sectorBandwidthPrice := he.host.UploadBandwidthPrice.Mul64(uint64(len(newData)))
	if sectorBandwidthPrice.Cmp(he.contract.LastRevision.NewValidProofOutputs[0].Value) >= 0 {
		return Contract{}, errors.New("contract has insufficient funds to support upload")
	}

	// calculate the new Merkle root
	newRoots := make([]crypto.Hash, len(he.contract.MerkleRoots))
	index := -1
	for i, h := range he.contract.MerkleRoots {
		if h == oldRoot {
			index = i
			newRoots[i] = newRoot
		} else {
			newRoots[i] = h
		}
	}
	if index == -1 {
		return Contract{}, errors.New("no record of that sector root")
	}
	merkleRoot := cachedMerkleRoot(newRoots)

	// create the action and revision
	actions := []modules.RevisionAction{{
		Type:        modules.ActionModify,
		SectorIndex: uint64(index),
		Offset:      offset,
		Data:        newData,
	}}
	rev := newModifyRevision(he.contract.LastRevision, merkleRoot, sectorBandwidthPrice)

	// run the revision iteration
	if err := he.runRevisionIteration(actions, rev, newRoots); err != nil {
		return Contract{}, err
	}

	// update metrics
	he.UploadSpending = he.UploadSpending.Add(sectorBandwidthPrice)

	return he.contract, nil
}

// NewEditor initiates the contract revision process with a host, and returns
// an Editor.
func NewEditor(host modules.HostDBEntry, contract Contract, currentHeight types.BlockHeight) (*Editor, error) {
	// check that contract has enough value to support an upload
	if len(contract.LastRevision.NewValidProofOutputs) != 2 {
		return nil, errors.New("invalid contract")
	}
	if !host.StoragePrice.IsZero() {
		bytes, errOverflow := contract.LastRevision.NewValidProofOutputs[0].Value.Div(host.StoragePrice).Uint64()
		if errOverflow == nil && bytes < modules.SectorSize {
			return nil, errors.New("contract has insufficient capacity")
		}
	}

	// initiate revision loop
	conn, err := net.DialTimeout("tcp", string(contract.IP), 15*time.Second)
	if err != nil {
		return nil, err
	}
	// allot 2 minutes for RPC request + revision exchange
	extendDeadline(conn, modules.NegotiateRecentRevisionTime)
	defer extendDeadline(conn, time.Hour)
	if err := encoding.WriteObject(conn, modules.RPCReviseContract); err != nil {
		return nil, errors.New("couldn't initiate RPC: " + err.Error())
	}
	if err := verifyRecentRevision(conn, contract); err != nil {
		return nil, errors.New("revision exchange failed: " + err.Error())
	}

	// the host is now ready to accept revisions
	return &Editor{
		host:     host,
		height:   currentHeight,
		contract: contract,
		conn:     conn,
	}, nil
}
