package contractor

import (
	"bytes"
	"errors"
	"net"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/types"
)

const (
	// SectorSize is the number of bytes in a sector.
	SectorSize = 1 << 22 // 4 MiB
)

// An Editor modifies a Contract by communicating with a host.
type Editor interface {
	// Upload revises the underlying contract to store the new data. It
	// returns the offset of the data in the stored file.
	Upload(data []byte) (offset uint64, err error)

	// Delete removes a sector from the underlying contract.
	Delete(crypto.Hash) error

	// Address returns the address of the host.
	Address() modules.NetAddress

	// ContractID returns the FileContractID of the contract.
	ContractID() types.FileContractID

	// EndHeight returns the height at which the contract ends.
	EndHeight() types.BlockHeight

	// Close terminates the connection to the uploader.
	Close() error
}

// A hostEditor modifies a Contract by calling the revise RPC on a host. It
// implements the Editor interface. hostEditors are NOT thread-safe; calls to
// Upload must happen in serial.
type hostEditor struct {
	// constants
	price types.Currency

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

// Close cleanly ends the revision process with the host, closes the
// connection, and submits the last revision to the transaction pool.
func (he *hostEditor) Close() error {
	// send an empty revision to indicate that we are finished
	encoding.WriteObject(he.conn, types.Transaction{})
	return he.conn.Close()
}

// Upload revises an existing file contract with a host, and then uploads a
// piece to it.
func (he *hostEditor) Upload(data []byte) (uint64, error) {
	// offset is old filesize
	offset := he.contract.LastRevision.NewFileSize

	// calculate price
	he.contractor.mu.RLock()
	height := he.contractor.blockHeight
	he.contractor.mu.RUnlock()
	if height > he.contract.FileContract.WindowStart {
		return 0, errors.New("contract has already ended")
	}
	piecePrice := types.NewCurrency64(uint64(len(data))).Mul(types.NewCurrency64(uint64(he.contract.FileContract.WindowStart - height))).Mul(he.price)

	// calculate the Merkle root of the new data (no error possible with bytes.Reader)
	pieceRoot, _ := crypto.ReaderMerkleRoot(bytes.NewReader(data))

	// calculate the new total Merkle root
	tree := crypto.NewCachedTree(0) // height is not relevant here
	for _, h := range he.contract.MerkleRoots {
		tree.Push(h[:])
	}
	tree.Push(pieceRoot[:])
	merkleRoot := tree.Root()

	// revise the file contract
	rev := newRevision(he.contract.LastRevision, uint64(len(data)), merkleRoot, piecePrice)
	signedTxn, err := negotiateRevision(he.conn, rev, data, he.contract.SecretKey)
	if err != nil {
		return 0, err
	}

	// update host contract
	he.contract.LastRevision = rev
	he.contract.LastRevisionTxn = signedTxn
	he.contract.MerkleRoots = append(he.contract.MerkleRoots, pieceRoot)

	he.contractor.mu.Lock()
	he.contractor.contracts[he.contract.ID] = he.contract
	he.contractor.save()
	he.contractor.mu.Unlock()

	return offset, nil
}

// Delete deletes a sector in a contract.
// TODO: implement
func (he *hostEditor) Delete(root crypto.Hash) error {
	// calculate the new total Merkle root
	// var newRoots []crypto.Hash
	// for _, h := range he.contract.MerkleRoots {
	// 	if h != root {
	// 		newRoots = append(newRoots, h)
	// 	}
	// }
	// tree := crypto.NewCachedTree(0) // height is not relevant here
	// for _, h := range newRoots {
	// 	tree.Push(h[:])
	// }
	// merkleRoot := tree.Root()

	// // send 'delete' action, sector root, and new Merkle root
	// encoding.WriteObject(he.conn, modules.RPCDelete)
	// encoding.WriteObject(he.conn, root)
	// encoding.WriteObject(he.conn, merkleRoot)

	// // read ok
	// encoding.ReadObject(he.conn, &ok)

	// // update host contract
	// he.contract.LastRevision = rev
	// he.contract.LastRevisionTxn = signedTxn
	// he.contract.MerkleRoots = newRoots

	// he.contractor.mu.Lock()
	// he.contractor.contracts[he.contract.ID] = he.contract
	// he.contractor.save()
	// he.contractor.mu.Unlock()

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
	settings, ok := c.hdb.Host(contract.IP)
	if !ok {
		return nil, errors.New("no record of that host")
	}
	if settings.Price.Cmp(maxPrice) > 0 {
		return nil, errTooExpensive
	}

	// check that contract has enough value to support an upload
	if len(contract.LastRevision.NewValidProofOutputs) != 2 {
		return nil, errors.New("invalid contract")
	}
	if !settings.Price.IsZero() {
		bytes, errOverflow := contract.LastRevision.NewValidProofOutputs[0].Value.Div(settings.Price).Uint64()
		if errOverflow == nil && bytes < SectorSize {
			return nil, errors.New("contract has insufficient capacity")
		}
	}

	// initiate revision loop
	conn, err := c.dialer.DialTimeout(contract.IP, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if err := encoding.WriteObject(conn, modules.RPCRevise); err != nil {
		return nil, err
	}
	if err := encoding.WriteObject(conn, contract.ID); err != nil {
		return nil, err
	}
	// TODO: some sort of acceptance would be good here, so that we know the
	// Editor will actually work. Maybe send the Merkle root?

	he := &hostEditor{
		contract: contract,
		price:    settings.Price,

		conn:       conn,
		contractor: c,
	}

	return he, nil
}
