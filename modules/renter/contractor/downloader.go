package contractor

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules/renter/contractor/proto"
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
	downloader *proto.Downloader
	contractor *Contractor
}

// Sector retrieves the sector with the specified Merkle root, and revises
// the underlying contract to pay the host proportionally to the data
// retrieve.
func (hd *hostDownloader) Sector(root crypto.Hash) ([]byte, error) {
	oldSpending := hd.downloader.DownloadSpending
	contract, sector, err := hd.downloader.Sector(root)
	if err != nil {
		return nil, err
	}
	delta := hd.downloader.DownloadSpending.Sub(oldSpending)

	hd.contractor.mu.Lock()
	hd.contractor.downloadSpending = hd.contractor.downloadSpending.Add(delta)
	hd.contractor.contracts[contract.ID] = contract
	hd.contractor.saveSync()
	hd.contractor.mu.Unlock()

	return sector, nil
}

// Close cleanly terminates the download loop with the host and closes the
// connection.
func (hd *hostDownloader) Close() error { return hd.downloader.Close() }

// Downloader initiates the download request loop with a host, and returns a
// Downloader.
func (c *Contractor) Downloader(contract proto.Contract) (Downloader, error) {
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
	if host.DownloadBandwidthPrice.Cmp(maxDownloadPrice) > 0 {
		return nil, errTooExpensive
	}

	// create downloader
	d, err := proto.NewDownloader(host, contract)
	if err != nil {
		return nil, err
	}

	return &hostDownloader{
		downloader: d,
		contractor: c,
	}, nil
}
