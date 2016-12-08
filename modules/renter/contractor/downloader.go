package contractor

import (
	"errors"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/proto"
	"github.com/NebulousLabs/Sia/types"
)

var errInvalidDownloader = errors.New("downloader has been invalidated because its contract is being renewed")

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
	clients      int // safe to Close when 0
	contractID   types.FileContractID
	contractor   *Contractor
	downloader   *proto.Downloader
	hostSettings modules.HostExternalSettings
	invalid      bool   // true if invalidate has been called
	speed        uint64 // Bytes per second.
	mu           sync.Mutex
}

// invalidate sets the invalid flag and closes the underlying
// proto.Downloader. Once invalidate returns, the hostDownloader is guaranteed
// to not further revise its contract. This is used during contract renewal to
// prevent a Downloader from revising a contract mid-renewal.
func (hd *hostDownloader) invalidate() {
	hd.mu.Lock()
	defer hd.mu.Unlock()
	hd.downloader.Close()
	hd.invalid = true
	hd.contractor.mu.Lock()
	delete(hd.contractor.downloaders, hd.contractID)
	delete(hd.contractor.revising, hd.contractID)
	hd.contractor.mu.Unlock()
}

// HostSettings returns the settings of the host that the downloader connects
// to.
func (hd *hostDownloader) HostSettings() modules.HostExternalSettings {
	hd.mu.Lock()
	defer hd.mu.Unlock()
	return hd.hostSettings
}

// Sector retrieves the sector with the specified Merkle root, and revises
// the underlying contract to pay the host proportionally to the data
// retrieve.
func (hd *hostDownloader) Sector(root crypto.Hash) ([]byte, error) {
	hd.mu.Lock()
	defer hd.mu.Unlock()
	if hd.invalid {
		return nil, errInvalidDownloader
	}
	oldSpending := hd.downloader.DownloadSpending
	start := time.Now()
	contract, sector, err := hd.downloader.Sector(root)
	duration := time.Since(start)
	if err != nil {
		return nil, err
	}
	delta := hd.downloader.DownloadSpending.Sub(oldSpending)

	hd.speed = uint64(duration.Seconds()) / modules.SectorSize

	c := hd.contractor
	c.mu.Lock()
	metrics := c.contractMetrics[contract.ID]
	metrics.DownloadSpending = metrics.DownloadSpending.Add(delta)
	metrics.Unspent = metrics.Unspent.Sub(delta)
	c.contractMetrics[contract.ID] = metrics
	c.financialMetrics.DownloadSpending = c.financialMetrics.DownloadSpending.Add(delta)
	c.contracts[contract.ID] = contract
	c.saveSync()
	c.mu.Unlock()

	return sector, nil
}

// Speed returns the most recent download speed of this host, in bytes per
// second.
func (hd *hostDownloader) Speed() uint64 {
	hd.mu.Lock()
	defer hd.mu.Unlock()
	return hd.speed
}

// Close cleanly terminates the download loop with the host and closes the
// connection.
func (hd *hostDownloader) Close() error {
	hd.mu.Lock()
	defer hd.mu.Unlock()
	hd.clients--
	// Close is a no-op if invalidate has been called, or if there are other
	// clients still using the hostDownloader.
	if hd.invalid || hd.clients > 0 {
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
	id = c.resolveID(id)
	cachedDownloader, haveDownloader := c.downloaders[id]
	height := c.blockHeight
	contract, haveContract := c.contracts[id]
	renewing := c.renewing[id]
	c.mu.RUnlock()

	if renewing {
		return nil, errors.New("currently renewing that contract")
	}

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
		clients:      1,
		contractID:   contract.ID,
		contractor:   c,
		downloader:   d,
		hostSettings: host.HostExternalSettings,
	}
	c.mu.Lock()
	c.downloaders[contract.ID] = hd
	c.mu.Unlock()

	return hd, nil
}
