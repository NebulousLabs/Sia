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

// An Editor modifies a Contract by communicating with a host. It uses the
// contract revision protocol to send modification requests to the host.
// Editors are the means by which the renter uploads data to hosts.
type Editor interface {
	// Upload revises the underlying contract to store the new data. It
	// returns the Merkle root of the data.
	Upload(data []byte) (root crypto.Hash, err error)

	// Delete removes a sector from the underlying contract.
	Delete(crypto.Hash) error

	// Modify overwrites a sector with new data. Because the Editor does not
	// have access to the original sector data, the new Merkle root must be
	// supplied by the caller.
	Modify(oldRoot, newRoot crypto.Hash, offset uint64, newData []byte) error

	// Address returns the address of the host.
	Address() modules.NetAddress

	// ContractID returns the FileContractID of the contract.
	ContractID() types.FileContractID

	// EndHeight returns the height at which the contract ends.
	EndHeight() types.BlockHeight

	// Close terminates the connection to the host.
	Close() error
}

// A hostEditor modifies a Contract by calling the revise RPC on a host. It
// implements the Editor interface. hostEditors are NOT thread-safe; calls to
// Upload must happen in serial.
type hostEditor struct {
	// constants
	host modules.HostDBEntry

	// updated after each revision
	contract Contract

	// resources
	conn       net.Conn
	contractor *Contractor
}

// Address returns the NetAddress of the host.
func (he *hostEditor) Address() modules.NetAddress { return he.contract.IP }

// ContractID returns the ID of the contract being revised.
func (he *hostEditor) ContractID() types.FileContractID { return he.contract.ID }

// EndHeight returns the height at which the host is no longer obligated to
// store the file.
func (he *hostEditor) EndHeight() types.BlockHeight { return he.contract.FileContract.WindowStart }

// Close cleanly terminates the revision loop with the host and closes the
// connection.
func (he *hostEditor) Close() error {
	// don't care about these errors
	_, _ = verifySettings(he.conn, he.host, he.contractor.hdb)
	_ = modules.WriteNegotiationStop(he.conn)
	return he.conn.Close()
}

// runRevisionIteration submits actions and their accompanying revision to the
// host for approval. If negotiation is successful, it updates the underlying
// Contract.
func (he *hostEditor) runRevisionIteration(actions []modules.RevisionAction, rev types.FileContractRevision, newRoots []crypto.Hash, height types.BlockHeight) error {
	// initiate revision
	if err := startRevision(he.conn, he.host, he.contractor.hdb); err != nil {
		return err
	}

	// send actions
	if err := encoding.WriteObject(he.conn, actions); err != nil {
		return err
	}

	// send revision to host and exchange signatures
	signedTxn, err := negotiateRevision(he.conn, rev, he.contract.SecretKey, height)
	if err != nil {
		return err
	}

	// update host contract
	he.contract.LastRevision = rev
	he.contract.LastRevisionTxn = signedTxn
	he.contract.MerkleRoots = newRoots

	he.contractor.mu.Lock()
	he.contractor.contracts[he.contract.ID] = he.contract
	he.contractor.save()
	he.contractor.mu.Unlock()

	return nil
}

// Upload negotiates a revision that adds a sector to a file contract.
func (he *hostEditor) Upload(data []byte) (crypto.Hash, error) {
	// allot 10 minutes for this exchange; sufficient to transfer 4 MB over 50 kbps
	he.conn.SetDeadline(time.Now().Add(600 * time.Second))
	defer he.conn.SetDeadline(time.Now().Add(time.Hour)) // reset deadline

	// get current height
	he.contractor.mu.RLock()
	height := he.contractor.blockHeight
	he.contractor.mu.RUnlock()
	if height >= he.contract.FileContract.WindowStart {
		return crypto.Hash{}, errors.New("contract has already ended")
	}

	// calculate price
	blockBytes := types.NewCurrency64(modules.SectorSize * uint64(he.contract.FileContract.WindowEnd-height))
	sectorStoragePrice := he.host.StoragePrice.Mul(blockBytes)
	sectorBandwidthPrice := he.host.UploadBandwidthPrice.Mul(types.NewCurrency64(modules.SectorSize))
	sectorPrice := sectorStoragePrice.Add(sectorBandwidthPrice)
	if sectorPrice.Cmp(he.contract.LastRevision.NewValidProofOutputs[0].Value) >= 0 {
		return crypto.Hash{}, errors.New("contract has insufficient funds to support upload")
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
	if err := he.runRevisionIteration(actions, rev, newRoots, height); err != nil {
		return crypto.Hash{}, err
	}

	return sectorRoot, nil
}

// Delete negotiates a revision that removes a sector from a file contract.
func (he *hostEditor) Delete(root crypto.Hash) error {
	// allot 2 minutes for this exchange
	he.conn.SetDeadline(time.Now().Add(120 * time.Second))
	defer he.conn.SetDeadline(time.Now().Add(time.Hour))

	// get current height
	he.contractor.mu.RLock()
	height := he.contractor.blockHeight
	he.contractor.mu.RUnlock()
	if height >= he.contract.FileContract.WindowStart {
		return errors.New("contract has already ended")
	}

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
		return errors.New("no record of that sector root")
	}
	merkleRoot := cachedMerkleRoot(newRoots)

	// create the action and accompanying revision
	actions := []modules.RevisionAction{{
		Type:        modules.ActionDelete,
		SectorIndex: uint64(index),
	}}
	rev := newRevision(he.contract.LastRevision, merkleRoot, uint64(len(newRoots)), sectorPrice, sectorCollateral)

	// run the revision iteration
	return he.runRevisionIteration(actions, rev, newRoots, height)
}

// Modify negotiates a revision that edits a sector in a file contract.
func (he *hostEditor) Modify(oldRoot, newRoot crypto.Hash, offset uint64, newData []byte) error {
	// allot 10 minutes for this exchange; sufficient to transfer 4 MB over 50 kbps
	he.conn.SetDeadline(time.Now().Add(600 * time.Second))
	defer he.conn.SetDeadline(time.Now().Add(time.Hour)) // reset deadline

	// get current height
	he.contractor.mu.RLock()
	height := he.contractor.blockHeight
	he.contractor.mu.RUnlock()
	if height >= he.contract.FileContract.WindowStart {
		return errors.New("contract has already ended")
	}

	// calculate price
	sectorPrice := he.host.UploadBandwidthPrice.Mul(types.NewCurrency64(uint64(len(newData))))
	if sectorPrice.Cmp(he.contract.LastRevision.NewValidProofOutputs[0].Value) >= 0 {
		return errors.New("contract has insufficient funds to support upload")
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
		return errors.New("no record of that sector root")
	}
	merkleRoot := cachedMerkleRoot(newRoots)

	// create the action and revision
	actions := []modules.RevisionAction{{
		Type:        modules.ActionModify,
		SectorIndex: uint64(index),
		Offset:      offset,
		Data:        newData,
	}}
	rev := newModifyRevision(he.contract.LastRevision, merkleRoot, sectorPrice)

	// run the revision iteration
	if err := he.runRevisionIteration(actions, rev, newRoots, height); err != nil {
		return err
	}

	return nil
}

// Editor initiates the contract revision process with a host, and returns
// an Editor.
func (c *Contractor) Editor(contract Contract) (Editor, error) {
	c.mu.RLock()
	height := c.blockHeight
	c.mu.RUnlock()
	if height > contract.FileContract.WindowStart {
		return nil, errors.New("contract has already ended")
	}
	host, ok := c.hdb.Host(contract.IP)
	if !ok {
		return nil, errors.New("no record of that host")
	}
	if host.StoragePrice.Cmp(maxPrice) > 0 {
		return nil, errTooExpensive
	}

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
	conn, err := c.dialer.DialTimeout(contract.IP, 15*time.Second)
	if err != nil {
		return nil, err
	}
	// allot 2 minutes for RPC request + revision exchange
	conn.SetDeadline(time.Now().Add(120 * time.Second))
	defer conn.SetDeadline(time.Now().Add(time.Hour))
	if err := encoding.WriteObject(conn, modules.RPCReviseContract); err != nil {
		return nil, errors.New("couldn't initiate RPC: " + err.Error())
	}
	if err := verifyRecentRevision(conn, contract); err != nil {
		return nil, errors.New("revision exchange failed: " + err.Error())
	}

	// the host is now ready to accept revisions
	he := &hostEditor{
		contract: contract,
		host:     host,

		conn:       conn,
		contractor: c,
	}

	return he, nil
}
