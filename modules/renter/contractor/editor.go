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

// the contractor will cap host's MaxCollateral setting to this value
var maxUploadCollateral = types.SiacoinPrecision.Mul64(1e3).Div(modules.BlockBytesPerMonthTerabyte) // 1k SC / TB / Month

// An Editor modifies a Contract by communicating with a host. It uses the
// contract revision protocol to send modification requests to the host.
// Editors are the means by which the renter uploads data to hosts.
type Editor interface {
	// Upload revises the underlying contract to store the new data. It
	// returns the Merkle root of the data.
	Upload(data []byte) (root crypto.Hash, err error)

	// Delete removes a sector from the underlying contract.
	Delete(crypto.Hash) error

	// Modify overwrites a sector with new data. Because the Editor does not
	// have access to the original sector data, the new Merkle root must be
	// supplied by the caller.
	Modify(oldRoot, newRoot crypto.Hash, offset uint64, newData []byte) error

	// Address returns the address of the host.
	Address() modules.NetAddress

	// ContractID returns the FileContractID of the contract.
	ContractID() types.FileContractID

	// EndHeight returns the height at which the contract ends.
	EndHeight() types.BlockHeight

	// Close terminates the connection to the host.
	Close() error
}

// A hostEditor modifies a Contract by calling the revise RPC on a host. It
// implements the Editor interface. hostEditors are safe for use by
// multiple goroutines.
type hostEditor struct {
	clients    int // safe to Close when 0
	contract   modules.RenterContract
	contractor *Contractor
	editor     *proto.Editor
	mu         sync.Mutex
}

// Address returns the NetAddress of the host.
func (he *hostEditor) Address() modules.NetAddress { return he.contract.NetAddress }

// ContractID returns the ID of the contract being revised.
func (he *hostEditor) ContractID() types.FileContractID { return he.contract.ID }

// EndHeight returns the height at which the host is no longer obligated to
// store the file.
func (he *hostEditor) EndHeight() types.BlockHeight { return he.contract.EndHeight() }

// Close cleanly terminates the revision loop with the host and closes the
// connection.
func (he *hostEditor) Close() error {
	he.mu.Lock()
	defer he.mu.Unlock()
	he.clients--
	// Close is a no-op if invalidate has been called, or if there are other
	// clients still using the hostEditor.
	if he.clients > 0 {
		return nil
	}
	he.contractor.mu.Lock()
	delete(he.contractor.editors, he.contract.ID)
	he.contractor.mu.Unlock()
	he.contractor.managedUnlockContract(he.contract.ID)
	return he.editor.Close()
}

// Upload negotiates a revision that adds a sector to a file contract.
func (he *hostEditor) Upload(data []byte) (crypto.Hash, error) {
	he.mu.Lock()
	defer he.mu.Unlock()
	contract, sectorRoot, err := he.editor.Upload(data)
	if err != nil {
		return crypto.Hash{}, err
	}
	he.contractor.mu.Lock()
	he.contractor.contracts[contract.ID] = contract
	he.contractor.persist.update(updateUploadRevision{
		NewRevisionTxn:     contract.LastRevisionTxn,
		NewSectorRoot:      sectorRoot,
		NewSectorIndex:     len(contract.MerkleRoots) - 1,
		NewUploadSpending:  contract.UploadSpending,
		NewStorageSpending: contract.StorageSpending,
	})
	he.contractor.mu.Unlock()
	he.contract = contract

	return sectorRoot, nil
}

// Delete negotiates a revision that removes a sector from a file contract.
func (he *hostEditor) Delete(root crypto.Hash) error {
	he.mu.Lock()
	defer he.mu.Unlock()

	contract, err := he.editor.Delete(root)
	if err != nil {
		return err
	}

	he.contractor.mu.Lock()
	he.contractor.contracts[contract.ID] = contract
	he.contractor.saveSync()
	he.contractor.mu.Unlock()
	he.contract = contract

	return nil
}

// Modify negotiates a revision that edits a sector in a file contract.
func (he *hostEditor) Modify(oldRoot, newRoot crypto.Hash, offset uint64, newData []byte) error {
	he.mu.Lock()
	defer he.mu.Unlock()
	contract, err := he.editor.Modify(oldRoot, newRoot, offset, newData)
	if err != nil {
		return err
	}
	he.contractor.mu.Lock()
	he.contractor.contracts[contract.ID] = contract
	he.contractor.saveSync()
	he.contractor.mu.Unlock()
	he.contract = contract

	return nil
}

// Editor returns a Editor object that can be used to upload, modify, and
// delete sectors on a host.
func (c *Contractor) Editor(id types.FileContractID, cancel <-chan struct{}) (_ Editor, err error) {
	c.mu.RLock()
	id = c.ResolveID(id)
	cachedEditor, haveEditor := c.editors[id]
	height := c.blockHeight
	contract, haveContract := c.contracts[id]
	c.mu.RUnlock()

	if haveEditor {
		// increment number of clients and return
		cachedEditor.mu.Lock()
		cachedEditor.clients++
		cachedEditor.mu.Unlock()
		return cachedEditor, nil
	}

	host, haveHost := c.hdb.Host(contract.HostPublicKey)
	if !haveContract {
		return nil, errors.New("no record of that contract")
	} else if height > contract.EndHeight() {
		return nil, errors.New("contract has already ended")
	} else if !haveHost {
		return nil, errors.New("no record of that host")
	}
	contract.NetAddress = host.NetAddress

	// Acquire contract lock.
	if !c.managedTryLockContract(id) {
		return nil, errors.New("contract is in use elsewhere, unable to create an editor")
	}
	// Release lock early if function has an error before returning.
	defer func() {
		if err != nil {
			c.managedUnlockContract(id)
		}
	}()

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

	// create editor
	e, err := proto.NewEditor(host, contract, height, cancel)
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
		contract.MerkleRoots = cached.MerkleRoots
		e, err = proto.NewEditor(host, contract, height, cancel)
	}
	if err != nil {
		return nil, err
	}
	// supply a SaveFn that saves the revision to the contractor's persist
	// (the existing revision will be overwritten when SaveFn is called)
	e.SaveFn = c.saveUploadRevision(contract.ID)

	// cache editor
	he := &hostEditor{
		clients:    1,
		contract:   contract,
		contractor: c,
		editor:     e,
	}
	c.mu.Lock()
	c.editors[contract.ID] = he
	c.mu.Unlock()
	return he, nil
}
