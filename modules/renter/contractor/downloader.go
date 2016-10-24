package contractor

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/crypto"
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
// It implements the Downloader interface. hostDownloaders are safe for use by
// multiple goroutines.
type hostDownloader struct {
	clients    int // safe to Close when 0
	contractID types.FileContractID
	contractor *Contractor
	downloader *proto.Downloader
	mu         sync.Mutex
}

// Sector retrieves the sector with the specified Merkle root, and revises
// the underlying contract to pay the host proportionally to the data
// retrieve.
func (hd *hostDownloader) Sector(root crypto.Hash) ([]byte, error) {
	hd.mu.Lock()
	defer hd.mu.Unlock()

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
	hd.mu.Lock()
	defer hd.mu.Unlock()
	// Close is a no-op unless there is only one goroutine using the
	// hostDownloader
	hd.clients--
	if hd.clients > 0 {
		return nil
	}
	hd.contractor.mu.Lock()
	delete(hd.contractor.downloaders, hd.contractID)
	delete(hd.contractor.revising, hd.contractID)
	hd.contractor.mu.Unlock()
	return hd.downloader.Close()
}

// Downloader returns a Downloader object that can be used to download sectors
// from a host.
func (c *Contractor) Downloader(id types.FileContractID) (_ Downloader, err error) {
	c.mu.RLock()
	cachedDownloader, haveDownloader := c.downloaders[id]
	height := c.blockHeight
	contract, haveContract := c.contracts[id]
	c.mu.RUnlock()

	if haveDownloader {
		// increment number of clients and return
		cachedDownloader.mu.Lock()
		cachedDownloader.clients++
		cachedDownloader.mu.Unlock()
		return cachedDownloader, nil
	}

	host, haveHost := c.hdb.Host(contract.NetAddress)
	if !haveContract {
		return nil, errors.New("no record of that contract")
	} else if height > contract.EndHeight() {
		return nil, errors.New("contract has already ended")
	} else if !haveHost {
		return nil, errors.New("no record of that host")
	} else if host.DownloadBandwidthPrice.Cmp(maxDownloadPrice) > 0 {
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

	// release lock early if function returns an error
	defer func() {
		if err != nil {
			c.mu.Lock()
			delete(c.revising, contract.ID)
			c.mu.Unlock()
		}
	}()

	// create downloader
	d, err := proto.NewDownloader(host, contract)
	if proto.IsRevisionMismatch(err) {
		// try again with the cached revision
		c.mu.RLock()
		cached, ok := c.cachedRevisions[contract.ID]
		c.mu.RUnlock()
		if !ok {
			// nothing we can do; return original error
			return nil, err
		}
		c.log.Printf("host %v has different revision for %v; retrying with cached revision", contract.NetAddress, contract.ID)
		contract.LastRevision = cached.revision
		d, err = proto.NewDownloader(host, contract)
	}
	if err != nil {
		return nil, err
	}
	// supply a SaveFn that saves the revision to the contractor's persist
	// (the existing revision will be overwritten when SaveFn is called)
	d.SaveFn = c.saveRevision(contract.ID)

	// cache downloader
	hd := &hostDownloader{
		clients:    1,
		contractID: contract.ID,
		contractor: c,
		downloader: d,
	}
	c.mu.Lock()
	c.downloaders[contract.ID] = hd
	c.mu.Unlock()

	return hd, nil
}
