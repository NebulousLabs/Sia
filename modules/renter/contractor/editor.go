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

var errInvalidEditor = errors.New("editor has been invalidated because its contract is being renewed")

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
	contractor *Contractor
	editor     *proto.Editor
	endHeight  types.BlockHeight
	id         types.FileContractID
	invalid    bool // true if invalidate has been called
	netAddress modules.NetAddress

	mu sync.Mutex
}

// invalidate sets the invalid flag and closes the underlying proto.Editor.
// Once invalidate returns, the hostEditor is guaranteed to not further revise
// its contract. This is used during contract renewal to prevent an Editor
// from revising a contract mid-renewal.
func (he *hostEditor) invalidate() {
	he.mu.Lock()
	defer he.mu.Unlock()
	if !he.invalid {
		he.editor.Close()
		he.invalid = true
	}
	he.contractor.mu.Lock()
	delete(he.contractor.editors, he.id)
	delete(he.contractor.revising, he.id)
	he.contractor.mu.Unlock()
}

// Address returns the NetAddress of the host.
func (he *hostEditor) Address() modules.NetAddress { return he.netAddress }

// ContractID returns the ID of the contract being revised.
func (he *hostEditor) ContractID() types.FileContractID { return he.id }

// EndHeight returns the height at which the host is no longer obligated to
// store the file.
func (he *hostEditor) EndHeight() types.BlockHeight { return he.endHeight }

// Close cleanly terminates the revision loop with the host and closes the
// connection.
func (he *hostEditor) Close() error {
	he.mu.Lock()
	defer he.mu.Unlock()
	he.clients--
	// Close is a no-op if invalidate has been called, or if there are other
	// clients still using the hostEditor.
	if he.invalid || he.clients > 0 {
		return nil
	}
	he.invalid = true
	he.contractor.mu.Lock()
	delete(he.contractor.editors, he.id)
	delete(he.contractor.revising, he.id)
	he.contractor.mu.Unlock()
	return he.editor.Close()
}

// Delete negotiates a revision that removes a sector from a file contract.
func (he *hostEditor) Delete(root crypto.Hash) (err error) {
	he.mu.Lock()
	defer he.mu.Unlock()
	if he.invalid {
		return errInvalidEditor
	}

	// Perform the delete.
	_, err = he.editor.Delete(root)
	if err != nil {
		return err
	}

	// Save.
	he.contractor.mu.Lock()
	he.contractor.saveSync()
	he.contractor.mu.Unlock()
	return nil
}

// Modify negotiates a revision that edits a sector in a file contract.
func (he *hostEditor) Modify(oldRoot, newRoot crypto.Hash, offset uint64, newData []byte) (err error) {
	he.mu.Lock()
	defer he.mu.Unlock()
	if he.invalid {
		return errInvalidEditor
	}

	// Perform the modification.
	_, err = he.editor.Modify(oldRoot, newRoot, offset, newData)
	if err != nil {
		return err
	}

	// Save.
	he.contractor.mu.Lock()
	he.contractor.saveSync()
	he.contractor.mu.Unlock()
	return nil
}

// Upload negotiates a revision that adds a sector to a file contract.
func (he *hostEditor) Upload(data []byte) (_ crypto.Hash, err error) {
	he.mu.Lock()
	defer he.mu.Unlock()
	if he.invalid {
		return crypto.Hash{}, errInvalidEditor
	}

	// Perform the upload.
	newContract, sectorRoot, err := he.editor.Upload(data)
	if err != nil {
		return crypto.Hash{}, err
	}

	// Save.
	he.contractor.mu.Lock()
	he.contractor.persist.update(updateUploadRevision{
		NewRevisionTxn:     newContract.LastRevisionTxn,
		NewSectorRoot:      sectorRoot,
		NewSectorIndex:     len(newContract.MerkleRoots) - 1,
		NewUploadSpending:  newContract.UploadSpending,
		NewStorageSpending: newContract.StorageSpending,
	})
	he.contractor.mu.Unlock()
	return sectorRoot, nil
}

// Editor returns a Editor object that can be used to upload, modify, and
// delete sectors on a host.
func (c *Contractor) Editor(id types.FileContractID, cancel <-chan struct{}) (_ Editor, err error) {
	id = c.ResolveID(id)
	c.mu.RLock()
	cachedEditor, haveEditor := c.editors[id]
	height := c.blockHeight
	renewing := c.renewing[id]
	c.mu.RUnlock()

	if renewing {
		// Cannot use the editor if the contract is being renewed.
		return nil, errors.New("currently renewing that contract")
	} else if haveEditor {
		// This editor already exists. Mark that there are now two routines
		// using the editor, and then return the editor that already exists.
		cachedEditor.mu.Lock()
		cachedEditor.clients++
		cachedEditor.mu.Unlock()
		return cachedEditor, nil
	}

	// Check that the contract and host are both available, and run some brief
	// sanity checks to see that the host is not swindling us.
	contract, haveContract := c.contracts.View(id)
	if !haveContract {
		return nil, errors.New("no record of that contract")
	}
	host, haveHost := c.hdb.Host(contract.HostPublicKey)
	if height > contract.EndHeight() {
		return nil, errors.New("contract has already ended")
	} else if !haveHost {
		return nil, errors.New("no record of that host")
	} else if host.StoragePrice.Cmp(maxStoragePrice) > 0 {
		return nil, errTooExpensive
	} else if host.UploadBandwidthPrice.Cmp(maxUploadPrice) > 0 {
		return nil, errTooExpensive
	}

	// Acquire the revising lock.
	c.mu.Lock()
	alreadyRevising := c.revising[contract.ID]
	if alreadyRevising {
		c.mu.Unlock()
		return nil, errors.New("already revising that contract")
	}
	c.revising[contract.ID] = true
	c.mu.Unlock()
	// Release the revising lock early in the event of an error.
	defer func() {
		if err != nil {
			c.mu.Lock()
			delete(c.revising, contract.ID)
			c.mu.Unlock()
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

	// Create the editor.
	e, err := proto.NewEditor(host, contract.ID, c.contracts, height, c.hdb, cancel)
	if proto.IsRevisionMismatch(err) {
		// Our original revision does not match the host, try again using the
		// cached revision.
		c.mu.RLock()
		cached, ok := c.cachedRevisions[contract.ID]
		c.mu.RUnlock()
		if !ok {
			c.log.Printf("Wanted to recover contract %v with host %v, but no revision was cached", contract.ID, host.NetAddress)
			return nil, err
		}
		c.log.Printf("Host %v has different revision for %v; retrying with cached revision", host.NetAddress, contract.ID)
		contract, haveContract = c.contracts.Acquire(contract.ID)
		if !haveContract {
			c.log.Critical("contract set does not contain contract")
		}
		contract.LastRevision = cached.Revision
		contract.MerkleRoots = cached.MerkleRoots
		c.contracts.Return(contract)
		e, err = proto.NewEditor(host, contract.ID, c.contracts, height, c.hdb, cancel)
		// needs to be handled separately since a revision mismatch is not automatically a failed interaction
		if proto.IsRevisionMismatch(err) {
			c.hdb.IncrementFailedInteractions(host.PublicKey)
		}

		// TODO: Update the contract set to have the cached data. This is
		// nontrivial, as we know the cached stuff was working on a previous set
		// of the contract, which we no longer have :<
		//
		// TODO: Make sure these fixes get transplanted to the editor as well.
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
		contractor: c,
		editor:     e,
		endHeight:  contract.EndHeight(),
		id:         contract.ID,
		netAddress: host.NetAddress,
	}
	c.mu.Lock()
	c.editors[contract.ID] = he
	c.mu.Unlock()

	return he, nil
}
