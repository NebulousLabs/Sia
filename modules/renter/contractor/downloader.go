package contractor

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/build"
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
// It implements the Downloader interface. hostDownloaders are safe for use by
// multiple goroutines.
type hostDownloader struct {
	clients      int // safe to Close when 0
	contractID   types.FileContractID
	contractor   *Contractor
	downloader   *proto.Downloader
	hostSettings modules.HostExternalSettings
	speed        uint64 // Bytes per second.
	mu           sync.Mutex
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
	contract, sector, err := hd.downloader.Sector(root)
	if err != nil {
		return nil, err
	}

	hd.contractor.mu.Lock()
	hd.contractor.contracts[contract.ID] = contract
	hd.contractor.persist.update(updateDownloadRevision{
		NewRevisionTxn:      contract.LastRevisionTxn,
		NewDownloadSpending: contract.DownloadSpending,
	})
	hd.contractor.mu.Unlock()

	return sector, nil
}

// Close cleanly terminates the download loop with the host and closes the
// connection.
func (hd *hostDownloader) Close() error {
	hd.mu.Lock()
	defer hd.mu.Unlock()
	hd.clients--
	// Close is a no-op if there are still clients using the downloader.
	if hd.clients > 0 {
		return nil
	}

	hd.contractor.mu.Lock()
	delete(hd.contractor.downloaders, hd.contractID)
	hd.contractor.mu.Unlock()
	hd.contractor.managedUnlockContract(hd.contractID)
	return hd.downloader.Close()
}

// Downloader returns a Downloader object that can be used to download sectors
// from a host.
func (c *Contractor) Downloader(id types.FileContractID, cancel <-chan struct{}) (_ Downloader, err error) {
	c.mu.RLock()
	id = c.ResolveID(id)
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

	host, haveHost := c.hdb.Host(contract.HostPublicKey)
	if !haveContract {
		return nil, errors.New("no record of that contract")
	} else if height > contract.EndHeight() {
		return nil, errors.New("contract has already ended")
	} else if !haveHost {
		return nil, errors.New("no record of that host")
	}
	// Update the contract to the most recent net address for the host.
	contract.NetAddress = host.NetAddress

	// Sanity check, unless this is a brand new contract, a cached revision
	// should exist.
	if build.DEBUG && contract.LastRevision.NewRevisionNumber > 1 {
		c.mu.RLock()
		_, exists := c.cachedRevisions[contract.ID]
		c.mu.RUnlock()
		if !exists {
			c.log.Critical("Cached revision does not exist for contract.")
		}
	}

	// Grab a lock on the contract. If the contract is already in use by either
	// the renew loop or by an editor, return an error.
	if !c.managedTryLockContract(id) {
		return nil, errors.New("contract is in use elsewhere, unable to create a downloader")
	}
	// If there's an error in the below code, release the lock on the contract.
	defer func() {
		if err != nil {
			c.managedUnlockContract(id)
		}
	}()

	// create downloader
	protoDownloader, err := proto.NewDownloader(host, contract, cancel)
	if proto.IsRevisionMismatch(err) {
		// try again with the cached revision
		c.mu.RLock()
		cached, ok := c.cachedRevisions[contract.ID]
		c.mu.RUnlock()
		if !ok {
			// nothing we can do; return original error
			c.log.Printf("wanted to recover contract %v with host %v, but no revision was cached", contract.ID, contract.NetAddress)
			return nil, err
		}
		c.log.Printf("host %v has different revision for %v; retrying with cached revision", contract.NetAddress, contract.ID)
		contract.LastRevision = cached.Revision
		protoDownloader, err = proto.NewDownloader(host, contract, cancel)
	}
	if err != nil {
		return nil, err
	}
	// supply a SaveFn that saves the revision to the contractor's persist
	// (the existing revision will be overwritten when SaveFn is called)
	protoDownloader.SaveFn = c.saveDownloadRevision(contract.ID)

	// cache downloader
	hd := &hostDownloader{
		clients:      1,
		contractID:   contract.ID,
		contractor:   c,
		downloader:   protoDownloader,
		hostSettings: host.HostExternalSettings,
	}
	c.mu.Lock()
	c.downloaders[contract.ID] = hd
	c.mu.Unlock()
	return hd, nil
}
