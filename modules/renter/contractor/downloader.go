package contractor

import (
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
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
	downloader *proto.Downloader
	contractor *Contractor
	contractID types.FileContractID
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
	hd.contractor.financialMetrics.DownloadSpending = hd.contractor.financialMetrics.DownloadSpending.Add(delta)
	hd.contractor.contracts[contract.ID] = contract
	hd.contractor.saveSync()
	hd.contractor.mu.Unlock()

	return sector, nil
}

// Close cleanly terminates the download loop with the host and closes the
// connection.
func (hd *hostDownloader) Close() error {
	// release revising lock
	hd.contractor.mu.Lock()
	delete(hd.contractor.revising, hd.contractID)
	hd.contractor.mu.Unlock()
	return hd.downloader.Close()
}

// Downloader initiates the download request loop with a host, and returns a
// Downloader.
func (c *Contractor) Downloader(contract modules.RenterContract) (Downloader, error) {
	c.mu.RLock()
	height := c.blockHeight
	c.mu.RUnlock()
	if height > contract.EndHeight() {
		return nil, errors.New("contract has already ended")
	}
	host, ok := c.hdb.Host(contract.NetAddress)
	if !ok {
		return nil, errors.New("no record of that host")
	}
	if host.DownloadBandwidthPrice.Cmp(maxDownloadPrice) > 0 {
		return nil, errTooExpensive
	}

	// acquire revising lock
	c.mu.Lock()
	alreadyRevising := c.revising[contract.ID]
	if alreadyRevising {
		c.mu.Unlock()
		return nil, errors.New("already revising that contract")
	}
	c.revising[contract.ID] = true
	c.mu.Unlock()
	releaseLock := func() {
		c.mu.Lock()
		delete(c.revising, contract.ID)
		c.mu.Unlock()
	}

	// create downloader
	d, err := proto.NewDownloader(host, contract)
	if proto.IsRevisionMismatch(err) {
		// try again with the cached revision
		c.mu.RLock()
		cached, ok := c.cachedRevisions[contract.ID]
		c.mu.RUnlock()
		if !ok {
			// nothing we can do; return original error
			releaseLock()
			return nil, err
		}
		c.log.Printf("host %v has different revision for %v; retrying with cached revision", contract.NetAddress, contract.ID)
		contract.LastRevision = cached.revision
		d, err = proto.NewDownloader(host, contract)
	}
	if err != nil {
		releaseLock()
		return nil, err
	}
	// supply a SaveFn that saves the revision to the contractor's persist
	// (the existing revision will be overwritten when SaveFn is called)
	d.SaveFn = c.saveRevision(contract.ID)

	return &hostDownloader{
		downloader: d,
		contractor: c,
		contractID: contract.ID,
	}, nil
}
