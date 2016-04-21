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

// An Downloader retrieves sectors from with a host. It requests one sector at
// a time, and revises the file contract to transfer money to the host
// proportional to the data retrieved.
type Downloader interface {
	// Sector retrieves the sector with the specified Merkle root, and revises
	// the underlying contract to pay the host proportionally to the data
	// retrieve.
	Sector(root crypto.Hash) ([]byte, error)

	// Close terminates the connection to the host.
	Close() error
}

// A hostDownloader retrieves sectors by calling the download RPC on a host.
// It implements the Downloader interface. hostDownloaders are NOT thread-
// safe; calls to Sector must be serialized.
type hostDownloader struct {
	// constants
	host modules.HostDBEntry

	// updated after each revision
	contract Contract

	// resources
	conn       net.Conn
	contractor *Contractor
}

// Sector retrieves the sector with the specified Merkle root, and revises
// the underlying contract to pay the host proportionally to the data
// retrieve.
func (hd *hostDownloader) Sector(root crypto.Hash) ([]byte, error) {
	// calculate price
	hd.contractor.mu.RLock()
	height := hd.contractor.blockHeight
	hd.contractor.mu.RUnlock()
	if height >= hd.contract.FileContract.WindowStart {
		return nil, errors.New("contract has already ended")
	}
	sectorPrice := hd.host.DownloadBandwidthPrice.Mul(types.NewCurrency64(modules.SectorSize))
	if sectorPrice.Cmp(hd.contract.LastRevision.NewValidProofOutputs[0].Value) >= 0 {
		return nil, errors.New("contract has insufficient funds to support download")
	}

	// initiate download request by confirming host settings
	if err := startDownload(hd.conn, hd.host, hd.contractor.hdb); err != nil {
		return nil, err
	}

	// create revision and download request
	rev := newDownloadRevision(hd.contract.LastRevision, sectorPrice)
	requests := []modules.DownloadRequest{{
		MerkleRoot: root,
		Offset:     0,
		Length:     modules.SectorSize,
	}}

	// send revision and requests to host for approval
	signedTxn, err := negotiateDownloadRevision(hd.conn, rev, requests, hd.contract.SecretKey, height)
	if err != nil {
		return nil, err
	}

	// read sector data, completing one iteration of the download loop
	// TODO: optimize this
	var sectors [][]byte
	if err := encoding.ReadObject(hd.conn, &sectors, modules.SectorSize+8); err != nil {
		return nil, err
	} else if len(sectors) != 1 {
		return nil, errors.New("host did not send enough sectors")
	}
	sector := sectors[0]
	if uint64(len(sector)) != modules.SectorSize {
		return nil, errors.New("host did not send enough sector data")
	} else if crypto.MerkleRoot(sector) != root {
		return nil, errors.New("host sent bad sector data")
	}

	// update host contract
	hd.contract.LastRevision = rev
	hd.contract.LastRevisionTxn = signedTxn

	hd.contractor.mu.Lock()
	hd.contractor.contracts[hd.contract.ID] = hd.contract
	hd.contractor.save()
	hd.contractor.mu.Unlock()

	return sector, nil
}

// Close cleanly terminates the download loop with the host and closes the
// connection.
func (hd *hostDownloader) Close() error {
	// don't care about these errors
	_, _ = verifySettings(hd.conn, hd.host, hd.contractor.hdb)
	_ = modules.WriteNegotiationStop(hd.conn)
	return hd.conn.Close()
}

// Downloader initiates the download request loop with a host, and returns a
// Downloader.
func (c *Contractor) Downloader(contract Contract) (Downloader, error) {
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

	// check that contract has enough value to support a download
	if len(contract.LastRevision.NewValidProofOutputs) != 2 {
		return nil, errors.New("invalid contract")
	}
	if !host.DownloadBandwidthPrice.IsZero() {
		bytes, errOverflow := contract.LastRevision.NewValidProofOutputs[0].Value.Div(host.DownloadBandwidthPrice).Uint64()
		if errOverflow == nil && bytes < modules.SectorSize {
			return nil, errors.New("contract has insufficient funds to support download")
		}
	}

	// initiate download loop
	conn, err := c.dialer.DialTimeout(contract.IP, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if err := encoding.WriteObject(conn, modules.RPCDownload); err != nil {
		return nil, errors.New("couldn't initiate RPC: " + err.Error())
	}
	if err := verifyRecentRevision(conn, contract); err != nil {
		return nil, errors.New("revision exchange failed: " + err.Error())
	}

	// the host is now ready to accept revisions
	he := &hostDownloader{
		contract: contract,
		host:     host,

		conn:       conn,
		contractor: c,
	}

	return he, nil
}
