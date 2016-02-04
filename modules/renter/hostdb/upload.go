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
	contract hostContract

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
	piecePrice = piecePrice.MulFloat(1.02) // COMPATv0.4.8 -- hosts reject exact prices

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
	conn, err := hdb.dialer.DialTimeout(hc.IP, 15*time.Second)
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
	// details of the contracts to be formed
	filesize uint64
	duration types.BlockHeight

	hosts     []*hostUploader
	blacklist []modules.NetAddress
	hdb       *HostDB
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

	// First reuse existing connections.
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

	// Extend the exclude set with the hosts on the pool's blacklist and the
	// hosts we're already connected to.
	exclude = append(exclude, p.blacklist...)
	for _, h := range p.hosts {
		exclude = append(exclude, h.Address())
	}

	// Ask the hostdb for random hosts. We always ask for at least 10, to
	// avoid selecting the same uncooperative hosts over and over.
	ask := n * 2
	if ask < 10 {
		ask = 10
	}
	p.hdb.mu.Lock()
	randHosts := p.hdb.randomHosts(ask, exclude)
	p.hdb.mu.Unlock()

	// Form new contracts with the randomly-picked hosts. If a contract can't
	// be formed, add the host to the pool's blacklist.
	var errs []error
	for _, host := range randHosts {
		contract, err := p.hdb.newContract(host, p.filesize, p.duration)
		if err != nil {
			p.blacklist = append(p.blacklist, host.NetAddress)
			errs = append(errs, err)
			continue
		}
		hu, err := p.hdb.newHostUploader(contract)
		if err != nil {
			p.blacklist = append(p.blacklist, host.NetAddress)
			continue
		}
		hosts = append(hosts, hu)
		p.hosts = append(p.hosts, hu)
		if len(hosts) >= n {
			break
		}
	}
	// If all attempts failed, log the error.
	if len(errs) == len(randHosts) && len(errs) > 0 {
		// Log the last error, since early errors are more likely to be
		// host-specific.
		p.hdb.log.Println("couldn't form any host contracts:", errs[len(errs)-1])
	}
	return hosts
}

// NewPool returns an empty HostPool, unless the HostDB contains no hosts at
// all.
func (hdb *HostDB) NewPool(filesize uint64, duration types.BlockHeight) (HostPool, error) {
	hdb.mu.RLock()
	defer hdb.mu.RUnlock()
	if hdb.isEmpty() {
		return nil, errors.New("HostDB is empty")
	}
	return &pool{
		filesize: filesize,
		duration: duration,
		hdb:      hdb,
	}, nil
}
