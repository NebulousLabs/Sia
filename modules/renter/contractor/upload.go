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

// A hostUploader uploads pieces to a host. It implements the uploader
// interface. hostUploaders are NOT thread-safe; calls to Upload must happen
// in serial.
type hostUploader struct {
	// constants
	price types.Currency

	// updated after each revision
	contract Contract

	// resources
	conn       net.Conn
	contractor *Contractor
}

// Address returns the NetAddress of the host.
func (hu *hostUploader) Address() modules.NetAddress { return hu.contract.IP }

// ContractID returns the ID of the contract being revised.
func (hu *hostUploader) ContractID() types.FileContractID { return hu.contract.ID }

// EndHeight returns the height at which the host is no longer obligated to
// store the file.
func (hu *hostUploader) EndHeight() types.BlockHeight { return hu.contract.FileContract.WindowStart }

// Close cleanly ends the revision process with the host, closes the
// connection, and submits the last revision to the transaction pool.
func (hu *hostUploader) Close() error {
	// send an empty revision to indicate that we are finished
	encoding.WriteObject(hu.conn, types.Transaction{})
	return hu.conn.Close()
}

// Upload revises an existing file contract with a host, and then uploads a
// piece to it.
func (hu *hostUploader) Upload(data []byte) (uint64, error) {
	// offset is old filesize
	offset := hu.contract.LastRevision.NewFileSize

	// calculate price
	hu.contractor.mu.RLock()
	height := hu.contractor.blockHeight
	hu.contractor.mu.RUnlock()
	if height > hu.contract.FileContract.WindowStart {
		return 0, errors.New("contract has already ended")
	}
	piecePrice := types.NewCurrency64(uint64(len(data))).Mul(types.NewCurrency64(uint64(hu.contract.FileContract.WindowStart - height))).Mul(hu.price)

	// calculate the Merkle root of the new data (no error possible with bytes.Reader)
	pieceRoot, _ := crypto.ReaderMerkleRoot(bytes.NewReader(data))

	// calculate the new total Merkle root
	tree := crypto.NewCachedTree(0) // height is not relevant here
	for _, h := range hu.contract.MerkleRoots {
		tree.Push(h[:])
	}
	tree.Push(pieceRoot[:])
	merkleRoot := tree.Root()

	// revise the file contract
	rev := newRevision(hu.contract.LastRevision, uint64(len(data)), merkleRoot, piecePrice)
	signedTxn, err := negotiateRevision(hu.conn, rev, data, hu.contract.SecretKey)
	if err != nil {
		return 0, err
	}

	// update host contract
	hu.contract.LastRevision = rev
	hu.contract.LastRevisionTxn = signedTxn
	hu.contract.MerkleRoots = append(hu.contract.MerkleRoots, pieceRoot)

	hu.contractor.mu.Lock()
	hu.contractor.contracts[hu.contract.ID] = hu.contract
	hu.contractor.save()
	hu.contractor.mu.Unlock()

	return offset, nil
}

// Uploader initiates the contract revision process with a host, and returns
// an Uploader.
func (c *Contractor) Uploader(contract Contract) (Uploader, error) {
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
	// uploader will actually work. Maybe send the Merkle root?

	hu := &hostUploader{
		contract: contract,
		price:    settings.Price,

		conn:       conn,
		contractor: c,
	}

	return hu, nil
}
