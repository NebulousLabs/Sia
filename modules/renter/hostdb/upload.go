package hostdb

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
	tree     crypto.MerkleTree
	contract hostContract // only lastTxn is updated

	// resources
	conn net.Conn
	hdb  *HostDB
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
	hu.conn.Close()
	// submit the most recent revision to the blockchain
	err := hu.hdb.tpool.AcceptTransactionSet([]types.Transaction{hu.contract.LastRevisionTxn})
	if err != nil && err != modules.ErrDuplicateTransactionSet {
		hu.hdb.log.Println("WARN: transaction pool rejected revision transaction:", err)
	}
	return err
}

// Upload revises an existing file contract with a host, and then uploads a
// piece to it.
func (hu *hostUploader) Upload(data []byte) (uint64, error) {
	// offset is old filesize
	offset := hu.contract.LastRevision.NewFileSize

	// calculate price
	hu.hdb.mu.RLock()
	height := hu.hdb.blockHeight
	hu.hdb.mu.RUnlock()
	if height > hu.contract.FileContract.WindowStart {
		return 0, errors.New("contract has already ended")
	}
	piecePrice := types.NewCurrency64(uint64(len(data))).Mul(types.NewCurrency64(uint64(hu.contract.FileContract.WindowStart - height))).Mul(hu.price)

	// calculate new merkle root (no error possible with bytes.Reader)
	_ = hu.tree.ReadSegments(bytes.NewReader(data))
	merkleRoot := hu.tree.Root()

	// revise the file contract
	rev := newRevision(hu.contract.LastRevision, uint64(len(data)), merkleRoot, piecePrice)
	signedTxn, err := negotiateRevision(hu.conn, rev, data, hu.contract.SecretKey)
	if err != nil {
		return 0, err
	}

	// update host contract
	hu.contract.LastRevision = rev
	hu.contract.LastRevisionTxn = signedTxn
	hu.hdb.mu.Lock()
	hu.hdb.contracts[hu.contract.ID] = hu.contract
	hu.hdb.save()
	hu.hdb.mu.Unlock()

	return offset, nil
}

// newHostUploader initiates the contract revision process with a host, and
// returns a hostUploader, which satisfies the Uploader interface.
func (hdb *HostDB) newHostUploader(hc hostContract) (*hostUploader, error) {
	hdb.mu.RLock()
	settings, ok := hdb.allHosts[hc.IP] // or activeHosts?
	hdb.mu.RUnlock()
	if !ok {
		return nil, errors.New("no record of that host")
	}
	// TODO: check for excessive price again?

	// initiate revision loop
	conn, err := net.DialTimeout("tcp", string(hc.IP), 15*time.Second)
	if err != nil {
		return nil, err
	}
	if err := encoding.WriteObject(conn, modules.RPCRevise); err != nil {
		return nil, err
	}
	if err := encoding.WriteObject(conn, hc.ID); err != nil {
		return nil, err
	}
	// TODO: some sort of acceptance would be good here, so that we know the
	// uploader will actually work. Maybe send the Merkle root?

	hu := &hostUploader{
		contract: hc,
		price:    settings.Price,

		tree: crypto.NewTree(),

		conn: conn,
		hdb:  hdb,
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
		contract, err := p.hdb.newContract(host)
		if err != nil {
			continue
		}
		hu, err := p.hdb.newHostUploader(contract)
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
